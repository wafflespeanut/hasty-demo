package main

import (
	"log"
	"os"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
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
// - File store is always initialized.
// - If `S3_REGION` and `S3_BUCKET` is set, then AWS S3 store is initialized (unimplemented).
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
	objectStore := &FileStore{
		pathPrefix: storePathPrefix,
		openFds:    make(map[string]*os.File),
	}

	cmdHub := CommandHub{
		cmdChan:  make(chan repoMessage),
		respChan: make(chan interface{}),
		ackChan:  make(chan struct{}),
	}

	dataHub := ObjectHub{
		cmdChan:  make(chan repoMessage),
		respChan: make(chan interface{}),
		ackChan:  make(chan struct{}),
	}

	return &ImageRepository{
		linkCache,
		metaCache,
		dataStore,
		objectStore,
		cmdHub,
		dataHub,
	}, nil
}

// ImageRepository acts as the bridge between service handlers and the configured storage.
// It has a queue for asynchronously streaming images back and forth and has an LRU cache
// (with the configured capacity) for caching some of the data.
type ImageRepository struct {
	linkCache   *lru.Cache
	metaCache   *lru.Cache
	dataStore   DataStore
	objectStore ObjectStore
	cmdHub      CommandHub
	dataHub     ObjectHub
}

type repoCmdType int

// Internally used commands for querying/updating the repository.
const (
	cmdCreateUploadID = iota
	cmdFetchExpiry
	cmdAddImage
	chunkStore
)

// Message used within the repository.
type repoMessage struct {
	ty   repoCmdType
	id   string
	data interface{}
}

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
	return diff.Seconds() > 0
}

// sendChunk to the store. Since this goes through a channel all the way
// to the store, where it may be buffered before sending to the actual storage,
// the slice must retain its data. Hence, it's important to send a fresh copy
// of buffer. If we've reached EOF, then this must be called with an empty chunk.
func (r *ImageRepository) sendChunk(id string, chunk []byte) {
	r.dataHub.cmdChan <- repoMessage{
		ty:   chunkStore,
		id:   id,
		data: chunk,
	}
	_ = <-r.dataHub.ackChan
}

// CommandHub is responsible for passing commands from the service,
// handling them in the repository and getting the response.
type CommandHub struct {
	cmdChan  chan repoMessage
	respChan chan interface{}
	ackChan  chan struct{}
}

// handleCommands for this repository. NOTE: This is blocking and hence,
// it must be spawn into a separate goroutine.
func (r *ImageRepository) handleCommands() {
	for {
		select {
		case cmd := <-r.cmdHub.cmdChan:
			if cmd.ty == cmdCreateUploadID {
				r.linkCache.Add(cmd.id, cmd.data)
				r.cmdHub.ackChan <- struct{}{}
			} else if cmd.ty == cmdFetchExpiry {
				value, exists := r.linkCache.Get(cmd.id)
				if exists {
					r.cmdHub.respChan <- value
				} else {
					r.cmdHub.respChan <- defaultExpiry
				}
			}
		}
	}
}

// ObjectHub is responsible for proxying object chunks through this
// repository to/from the store.
type ObjectHub struct {
	cmdChan  chan repoMessage
	respChan chan interface{}
	ackChan  chan struct{}
}

// processChunks from store to the service and vice versa. NOTE: This is
// blocking and hence, it must be spawn into a separate goroutine.
func (r *ImageRepository) processChunks() {
	for {
		msg := <-r.dataHub.cmdChan
		if msg.ty == chunkStore {
			bytes := msg.data.([]byte)
			r.objectStore.storeChunk(msg.id, bytes, len(bytes) == 0)
			r.dataHub.ackChan <- struct{}{}
		}
	}
}
