package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/h2non/filetype"
	lru "github.com/hashicorp/golang-lru"
	"github.com/rwcarlsen/goexif/exif"
)

var (
	defaultExpiry = time.Unix(int64(1<<63-1), 0)
)

// Internally used commands for querying/updating the repository.
const (
	cmdCreateUploadID = iota
	cmdFetchExpiry
	cmdAddMeta
	cmdFetchMeta
	cmdFetchIDForHash
	cmdUpdateMeta
	cmdStoreChunk
	cmdFetchChunks
	cmdDiscardObject
	cmdAnalyzeImage
	cmdFetchStats
)

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

// Message used within the repository.
type repoMessage struct {
	ty   int
	id   string
	data interface{}
}

// MARK: API layer.

// DataRepository caches and proxies the database. In order for it to function,
// `handleCommands` must be spawned into a goroutine.
type DataRepository struct {
	linkCache *lru.Cache
	metaCache *lru.Cache
	hashes    *lru.Cache
	dataStore DataStore
	cmdHub    MessageHub
}

// NewDataRepository initialized from the environment and the given configuration parameters.
//
// If `POSTGRES_URL` is set, then the store for PostgreSQL database is initialized.
// Otherwise, a no-op store is initialized.
func NewDataRepository(linkCacheCap, metaCacheCap, hashesCap int) (*DataRepository, error) {
	linkCache, err := lru.New(linkCacheCap)
	if err != nil {
		return nil, err
	}

	metaCache, err := lru.New(metaCacheCap)
	if err != nil {
		return nil, err
	}

	hashes, err := lru.New(hashesCap)
	if err != nil {
		return nil, err
	}

	var dataStore DataStore

	postgresURL := os.Getenv(envPostgresURL)
	if postgresURL != "" {
		log.Println("Initializing PostgreSQL database driver.")
		dataStore = &PostgreSQLStore{
			url: postgresURL,
		}
	} else {
		log.Println("Initializing no-op store for metadata.")
		dataStore = NoOpStore{}
	}

	err = dataStore.initialize()
	if err != nil {
		return nil, err
	}

	cmdHub := NewMessageHub()

	return &DataRepository{
		linkCache,
		metaCache,
		hashes,
		dataStore,
		cmdHub,
	}, nil
}

// createUploadID binds the given ID to the given expiry time.
func (r *DataRepository) createUploadID(linkID string, expiry time.Time) {
	r.cmdHub.cmdChan <- repoMessage{
		ty:   cmdCreateUploadID,
		id:   linkID,
		data: expiry,
	}
	_ = <-r.cmdHub.ackChan
}

// hasExpired checks whether the given upload ID has expired.
func (r *DataRepository) hasExpired(linkID string) bool {
	r.cmdHub.cmdChan <- repoMessage{
		ty: cmdFetchExpiry,
		id: linkID,
	}
	expiry := <-r.cmdHub.respChan
	diff := expiry.(time.Time).Sub(time.Now().UTC())
	return diff.Seconds() <= 0
}

// fetchIDForHash of an image (if it exists, then we have a possible duplicate).
func (r *DataRepository) fetchIDForHash(hash string) string {
	r.cmdHub.cmdChan <- repoMessage{
		ty: cmdFetchIDForHash,
		id: hash,
	}
	value := <-r.cmdHub.respChan
	return value.(string)
}

// addImageData adds the given metadata for an image.
func (r *DataRepository) addImageData(meta ImageMeta) {
	r.cmdHub.cmdChan <- repoMessage{
		ty:   cmdAddMeta,
		data: meta,
	}
	_ = <-r.cmdHub.ackChan
}

// updateImageData updates existing metadata for an image.
func (r *DataRepository) updateImageData(meta ImageMeta) {
	r.cmdHub.cmdChan <- repoMessage{
		ty:   cmdUpdateMeta,
		data: meta,
	}
	_ = <-r.cmdHub.ackChan
}

// fetchImageMeta for the given image ID.
func (r *DataRepository) fetchImageMeta(id string) *ImageMeta {
	r.cmdHub.cmdChan <- repoMessage{
		ty: cmdFetchMeta,
		id: id,
	}
	value := <-r.cmdHub.respChan
	return value.(*ImageMeta)
}

// fetchStats collected from the service so far.
func (r *DataRepository) fetchStats() *ServiceStats {
	r.cmdHub.cmdChan <- repoMessage{
		ty: cmdFetchStats,
	}
	value := <-r.cmdHub.respChan
	return value.(*ServiceStats)
}

// handleCommands sent by the service.
//
// **NOTE:** This is blocking and hence, it must be spawn into a separate goroutine.
func (r *DataRepository) handleCommands() {
	// FIXME: Cleanup and isole commands handling into their own functions.
	for {
		cmd := <-r.cmdHub.cmdChan

		switch cmd.ty {
		case cmdCreateUploadID:
			expiry := cmd.data.(time.Time)
			r.linkCache.Add(cmd.id, expiry)
			r.dataStore.addUploadID(cmd.id, expiry)
			r.cmdHub.ackChan <- struct{}{}

		case cmdFetchExpiry:
			value, exists := r.linkCache.Get(cmd.id)
			if exists {
				r.cmdHub.respChan <- value
			} else {
				expiry, _ := r.dataStore.getUploadExpiry(cmd.id)
				if expiry != nil {
					r.linkCache.Add(cmd.id, *expiry)
					r.cmdHub.respChan <- *expiry
				} else {
					r.cmdHub.respChan <- defaultExpiry
				}
			}

		case cmdAddMeta:
			meta := cmd.data.(ImageMeta)
			r.metaCache.Add(meta.ID, meta)
			r.hashes.Add(meta.Hash, meta.ID)
			r.dataStore.addImageMeta(meta)
			r.cmdHub.ackChan <- struct{}{}

		case cmdFetchMeta:
			value, exists := r.metaCache.Get(cmd.id)
			if exists {
				meta := value.(ImageMeta)
				r.cmdHub.respChan <- &meta
			} else {
				meta, _ := r.dataStore.fetchImageMeta(cmd.id)
				if meta != nil {
					r.metaCache.Add(cmd.id, *meta)
					r.hashes.Add(meta.Hash, meta.ID)
				}
				r.cmdHub.respChan <- meta
			}

		case cmdFetchIDForHash:
			value, exists := r.hashes.Get(cmd.id)
			if exists {
				r.cmdHub.respChan <- value
			} else {
				meta, _ := r.dataStore.fetchMetaForHash(cmd.id)
				if meta != nil {
					r.metaCache.Add(cmd.id, *meta)
					r.hashes.Add(meta.Hash, meta.ID)
				}
				r.cmdHub.respChan <- meta.ID
			}

		case cmdUpdateMeta:
			meta := cmd.data.(ImageMeta)
			r.metaCache.Add(meta.ID, meta)
			r.hashes.Add(meta.Hash, meta.ID)
			r.dataStore.updateImageMeta(meta)
			r.cmdHub.ackChan <- struct{}{}

		case cmdFetchStats:
			stats, _ := r.dataStore.getServiceStats()
			r.cmdHub.respChan <- stats
		}
	}
}

// MARK: Streaming layer.

// ObjectsRepository acts as a bridge for streaming chunks from the service to the
// store and back.
type ObjectsRepository struct {
	objectStore ObjectStore
	data        *DataRepository
	streamHub   MessageHub
	imageHub    MessageHub
}

// NewObjectsRepository initialized from the environment and the DataRepository.
//
// - If `S3_REGION` and `S3_BUCKET` is set, then AWS S3 store is initialized (**unimplemented**).
// - Otherwise, file store is initialized (store path can be set in environment).
func NewObjectsRepository(data *DataRepository) (*ObjectsRepository, error) {
	log.Println("Initializing file store for images.")
	storePathPrefix := os.Getenv(envStorePath)
	if storePathPrefix == "" {
		storePathPrefix = defaultStorePath
	}

	storePathPrefix = strings.TrimSuffix(storePathPrefix, "/")
	err := os.MkdirAll(storePathPrefix, os.ModePerm)
	if err != nil {
		return nil, err
	}

	return &ObjectsRepository{
		data: data,
		objectStore: &FileStore{
			pathPrefix: storePathPrefix,
			openFds:    make(map[string]*os.File),
		},
		streamHub: NewMessageHub(),
		imageHub:  NewMessageHub(),
	}, nil
}

// sendChunk for the given image ID to the store. Since this goes through
// a channel all the way to the store, where it may be buffered before
// sending to the actual storage, the slice must retain its data. Hence,
// it's important to send a fresh copy of buffer. If we've reached EOF,
// then this must be called with an empty chunk.
func (r *ObjectsRepository) sendChunk(id string, chunk []byte) {
	r.streamHub.cmdChan <- repoMessage{
		ty:   cmdStoreChunk,
		id:   id,
		data: chunk,
	}
	_ = <-r.streamHub.ackChan
}

// fetchChunks for the given image ID and return a channel to stream them.
func (r *ObjectsRepository) fetchChunks(id string) <-chan Chunk {
	r.streamHub.cmdChan <- repoMessage{
		ty: cmdFetchChunks,
		id: id,
	}
	value := <-r.streamHub.respChan
	streamChan := value.(chan Chunk)
	return streamChan
}

// discardChunks corresponding to the given image ID.
func (r *ObjectsRepository) discardChunks(id string) {
	r.streamHub.cmdChan <- repoMessage{
		ty: cmdDiscardObject,
		id: id,
	}
	_ = <-r.streamHub.ackChan
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
func (r *ObjectsRepository) processChunks() {
	for {
		msg := <-r.streamHub.cmdChan

		switch msg.ty {
		case cmdStoreChunk:
			bytes := msg.data.([]byte)
			r.objectStore.storeChunk(msg.id, bytes, len(bytes) == 0)
			r.streamHub.ackChan <- struct{}{}

		case cmdFetchChunks:
			chunkChan := make(chan Chunk)
			// Spawn in a separate goroutine because we don't wanna
			// block other chunks whilst retrieving this file's chunks.
			go r.objectStore.retrieveChunks(msg.id, chunkChan)
			r.streamHub.respChan <- chunkChan

		case cmdDiscardObject:
			r.objectStore.discardObject(msg.id)
			r.streamHub.ackChan <- struct{}{}
		}
	}
}

// MARK: Processing layer

// queueImageForAnalysis using the given metadata.
func (r *ObjectsRepository) queueImageForAnalysis(meta ImageMeta) {
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
func (r *ObjectsRepository) processImages() {
	for {
		msg := <-r.imageHub.cmdChan

		switch msg.ty {
		case cmdAnalyzeImage:
			data := msg.data
			meta := data.(ImageMeta)
			meta.applyDefaults()

			r.updateMetaFromExif(&meta)
			r.updateFormat(&meta)

			log.Printf("Updating image (ID: %s, size: %d)\n", meta.ID, meta.Size)
			r.data.updateImageData(meta)
		}
	}
}

// updateMetaFromExif of the image in the given metadata.
func (r *ObjectsRepository) updateMetaFromExif(meta *ImageMeta) {
	reader, err := r.objectStore.getImageReader(meta.ID)
	if err != nil {
		log.Printf("Cannot obtain reader for getting image metadata (ID: %s): %s\n",
			meta.ID, err.Error())
		return
	}

	x, err := exif.Decode(reader)
	if err != nil {
		log.Printf("Cannot decode exif data from image (ID: %s): %s\n",
			meta.ID, err.Error())
		return
	}

	// NOTE: Right now, we're only worried about the camera model
	// and GPS coords, but we can always get more.

	value, err := x.Get(exif.Model)
	if err == nil {
		meta.CameraModel = value.String()
	}

	lat, long, err := x.LatLong()
	if err != nil {
		meta.Latitude = lat
		meta.Longitude = long
	}

	err = r.objectStore.cleanupImageReader(meta.ID, reader)
	if err != nil {
		log.Printf("Error cleaning up reader (id: %s): %s\n", meta.ID, err.Error())
	}
}

// updateFormat of the image in the given metadata.
func (r *ObjectsRepository) updateFormat(meta *ImageMeta) {
	reader, err := r.objectStore.getImageReader(meta.ID)
	if err != nil {
		log.Printf("Cannot obtain reader for checking image (ID: %s): %s\n",
			meta.ID, err.Error())
		return
	}

	kind, _ := filetype.MatchReader(reader)
	meta.MediaType = kind.MIME.Value
	// FIXME: If this isn't an image, then we should discard that in store.
}
