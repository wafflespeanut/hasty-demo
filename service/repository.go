package main

import (
	"log"
	"os"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/rwcarlsen/goexif/exif"
)

var (
	defaultExpiry = time.Unix(int64(1<<63-1), 0)
)

// initializeRepository from the environment and the given configuration parameters.
//
// The repository holds the stores for both metadata and the images themselves. Here,
// we use the environment to decide which store we'll be using.
//
// Data storage:
// - If `POSTGRES_URL` is set, then the store for PostgreSQL database is initialized (unimplemented).
// - Otherwise, a no-op store is initialized.
//
// Object storage:
// - If `S3_REGION` and `S3_BUCKET` is set, then AWS S3 store is initialized (unimplemented).
// - Otherwise, file store is initialized (store path can be set in environment).
func initializeRepository(linkCacheCap, metaCacheCap int) (*ImageRepository, error) {
	linkCache, err := lru.New(linkCacheCap)
	if err != nil {
		return nil, err
	}

	metaCache, err := lru.New(metaCacheCap)
	if err != nil {
		return nil, err
	}

	log.Println("Initializing no-op store for metadata.")
	dataStore := NoOpStore{}

	log.Println("Initializing file store for image.")
	storePathPrefix := os.Getenv(envStorePath)
	if storePathPrefix == "" {
		storePathPrefix = defaultStorePath
	}

	storePathPrefix = strings.TrimSuffix(storePathPrefix, "/")
	err = os.MkdirAll(storePathPrefix, os.ModePerm)
	if err != nil {
		return nil, err
	}

	objectStore := &FileStore{
		pathPrefix: storePathPrefix,
		openFds:    make(map[string]*os.File),
	}

	cmdHub := NewMessageHub()
	streamHub := NewMessageHub()
	imageHub := NewMessageHub()

	return &ImageRepository{
		linkCache,
		metaCache,
		dataStore,
		objectStore,
		cmdHub,
		streamHub,
		imageHub,
	}, nil
}

// FIXME: Repository should be split later for caching, processing and streaming.
// i.e., each hub should be part of its own repository.

// ImageRepository acts as the bridge between service handlers and the configured storage.
// It has a queue for asynchronously streaming images back and forth and has an LRU cache
// (with the configured capacity) for caching some of the data.
type ImageRepository struct {
	linkCache   *lru.Cache
	metaCache   *lru.Cache
	dataStore   DataStore
	objectStore ObjectStore
	cmdHub      MessageHub
	streamHub   MessageHub
	imageHub    MessageHub
}

// MessageHub has a bunch of channels for passing commands from the service,
// handling them in the repository, persisting/retrieving from the store, etc.
type MessageHub struct {
	cmdChan  chan repoMessage
	respChan chan interface{}
	ackChan  chan struct{}
}

// NewMessageHub for creating a new hub.
func NewMessageHub() MessageHub {
	return MessageHub{
		cmdChan:  make(chan repoMessage),
		respChan: make(chan interface{}),
		ackChan:  make(chan struct{}),
	}
}

type repoCmdType int

// Internally used commands for querying/updating the repository.
const (
	cmdCreateUploadID = iota
	cmdFetchExpiry
	cmdAddMeta
	cmdFetchMeta
	cmdUpdateMeta
	cmdStoreChunk
	cmdFetchChunks
	cmdAnalyzeImage
)

// Message used within the repository.
type repoMessage struct {
	ty   repoCmdType
	id   string
	data interface{}
}

// MARK: API layer.

// createUploadID binds the given ID to the given expiry time.
func (r *ImageRepository) createUploadID(linkID string, expiry time.Time) {
	r.cmdHub.cmdChan <- repoMessage{
		ty:   cmdCreateUploadID,
		id:   linkID,
		data: expiry,
	}
	_ = <-r.cmdHub.ackChan
}

// hasExpired checks whether the given upload ID has expired.
func (r *ImageRepository) hasExpired(linkID string) bool {
	r.cmdHub.cmdChan <- repoMessage{
		ty: cmdFetchExpiry,
		id: linkID,
	}
	expiry := <-r.cmdHub.respChan
	diff := expiry.(time.Time).Sub(time.Now().UTC())
	return diff.Seconds() <= 0
}

// addImageData adds the given metadata for an image.
func (r *ImageRepository) addImageData(meta ImageMeta) {
	r.cmdHub.cmdChan <- repoMessage{
		ty:   cmdAddMeta,
		data: meta,
	}
	_ = <-r.cmdHub.ackChan
}

// updateImageData updates existing metadata for an image.
func (r *ImageRepository) updateImageData(meta ImageMeta) {
	r.cmdHub.cmdChan <- repoMessage{
		ty:   cmdUpdateMeta,
		data: meta,
	}
	_ = <-r.cmdHub.ackChan
}

// fetchImageMeta for the given image ID.
func (r *ImageRepository) fetchImageMeta(id string) *ImageMeta {
	r.cmdHub.cmdChan <- repoMessage{
		ty: cmdFetchMeta,
		id: id,
	}
	value := <-r.cmdHub.respChan
	meta, ok := value.(ImageMeta)
	if !ok {
		return nil
	}

	return &meta
}

// handleCommands sent by the service.
//
// **NOTE:** This is blocking and hence, it must be spawn into a separate goroutine.
func (r *ImageRepository) handleCommands() {
	for {
		cmd := <-r.cmdHub.cmdChan
		if cmd.ty == cmdCreateUploadID {
			r.linkCache.Add(cmd.id, cmd.data)
			// FIXME: Persist in store.
			r.cmdHub.ackChan <- struct{}{}
		} else if cmd.ty == cmdFetchExpiry {
			value, exists := r.linkCache.Get(cmd.id)
			if exists {
				r.cmdHub.respChan <- value
			} else {
				// FIXME: Check store.
				r.cmdHub.respChan <- defaultExpiry
			}
		} else if cmd.ty == cmdAddMeta {
			meta := cmd.data.(ImageMeta)
			r.metaCache.Add(meta.ID, meta)
			// FIXME: Persist in store.
			r.cmdHub.ackChan <- struct{}{}
		} else if cmd.ty == cmdFetchMeta {
			value, exists := r.metaCache.Get(cmd.id)
			if exists {
				r.cmdHub.respChan <- value
			} else {
				// FIXME: Check store
				r.cmdHub.respChan <- nil
			}
		} else if cmd.ty == cmdUpdateMeta {
			meta := cmd.data.(ImageMeta)
			r.metaCache.Remove(meta.ID)
			r.metaCache.Add(meta.ID, meta)
			// FIXME: Persist in store.
			r.cmdHub.ackChan <- struct{}{}
		}
	}
}

// MARK: Streaming layer.

// sendChunk for the given image ID to the store. Since this goes through
// a channel all the way to the store, where it may be buffered before
// sending to the actual storage, the slice must retain its data. Hence,
// it's important to send a fresh copy of buffer. If we've reached EOF,
// then this must be called with an empty chunk.
func (r *ImageRepository) sendChunk(id string, chunk []byte) {
	r.streamHub.cmdChan <- repoMessage{
		ty:   cmdStoreChunk,
		id:   id,
		data: chunk,
	}
	_ = <-r.streamHub.ackChan
}

// fetchChunks for the given image ID and return a channel to stream them.
func (r *ImageRepository) fetchChunks(id string) <-chan Chunk {
	r.streamHub.cmdChan <- repoMessage{
		ty: cmdFetchChunks,
		id: id,
	}
	value := <-r.streamHub.respChan
	streamChan := value.(chan Chunk)
	return streamChan
}

// Chunk represents a streamable chunk.
type Chunk struct {
	bytes   []byte
	isFinal bool
	err     error
}

// processChunks from store to the service and vice versa.
//
// **NOTE:** This is blocking and hence, it must be spawn into a separate goroutine.
func (r *ImageRepository) processChunks() {
	for {
		msg := <-r.streamHub.cmdChan
		if msg.ty == cmdStoreChunk {
			bytes := msg.data.([]byte)
			r.objectStore.storeChunk(msg.id, bytes, len(bytes) == 0)
			r.streamHub.ackChan <- struct{}{}
		} else if msg.ty == cmdFetchChunks {
			chunkChan := make(chan Chunk)
			// Spawn in a separate goroutine because we don't wanna
			// block other chunks whilst retrieving this file's chunks.
			go r.objectStore.retrieveChunks(msg.id, chunkChan)
			r.streamHub.respChan <- chunkChan
		}
	}
}

// MARK: Processing layer

func (r *ImageRepository) queueImageForAnalysis(meta ImageMeta) {
	r.imageHub.cmdChan <- repoMessage{
		ty:   cmdAnalyzeImage,
		data: meta,
	}
}

// processImages we've stored so far. We've already done sanitation checks
// when we received the image, so now we just need to update the metadata
// by processing the tags.
//
// **NOTE:** This is blocking and hence, it must be spawn into a separate goroutine.
func (r *ImageRepository) processImages() {
	for {
		msg := <-r.imageHub.cmdChan
		if msg.ty == cmdAnalyzeImage {
			data := msg.data
			meta := data.(ImageMeta)
			meta.applyDefaults()

			reader, err := r.objectStore.getImageReader(meta.ID)
			if err != nil {
				log.Printf("Cannot obtain reader for image (ID: %s): %s\n", meta.ID, err.Error())
				r.updateImageData(meta)
				continue
			}

			x, err := exif.Decode(reader)
			if err != nil {
				log.Printf("Cannot decode exif data from image (ID: %s): %s\n", meta.ID, err.Error())
				r.updateImageData(meta)
				continue
			}

			// NOTE: Right now, we're only worried about the camera model,
			// but we can always get more.

			value, err := x.Get(exif.Model)
			if err == nil {
				meta.CameraModel = value.String()
			}

			err = r.objectStore.cleanupImageReader(meta.ID, reader)
			if err != nil {
				log.Printf("Error cleaning up reader (id: %s): %s\n", meta.ID, err.Error())
			}

			r.updateImageData(meta)
		}
	}
}
