package main

import (
	"log"
	"os"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
)

// initializeRepository from the environment and the given configuration parameters.
//
// The repository holds the stores for both metadata and the images themselves. Here,
// we use the environment to decide which store we'll be using.
//
// Data storage:
// - If `POSTGRES_URL` is set, then the store for PostgreSQL database is initialized.
// - Otherwise, a no-op store is initialized.
//
// Object storage:
// - If `S3_REGION` and `S3_BUCKET` is set, then AWS S3 store is initialized.
// - Otherwise, a file store is initialized.
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
	objectStore := FileStore{
		path: storePathPrefix,
	}

	linkChan := make(chan linkMessage)
	ackChan := make(chan struct{})

	return &ImageRepository{
		linkCache,
		metaCache,
		dataStore,
		objectStore,
		linkChan,
		ackChan,
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
	linkChan    chan linkMessage
	ackChan     chan struct{}
}

type repoCmdType int

// Internally used commands for querying/updating the repository.
const (
	cmdCreateUploadID = iota
	cmdAddImage
)

// Message used for exchanging upload link information.
type linkMessage struct {
	ty         repoCmdType
	id         string
	linkExpiry time.Time
}

// createUploadID binds the given ID to the given expiry time.
func (r *ImageRepository) createUploadID(linkID string, expiry time.Time) {
	r.linkChan <- linkMessage{
		ty:         cmdCreateUploadID,
		id:         linkID,
		linkExpiry: expiry,
	}
	_ = <-r.ackChan
}

// handleCommands for this repository. NOTE: This is blocking and hence,
// it must be spawn into a separate goroutine.
func (r *ImageRepository) handleCommands() {
	for {
		select {
		case cmd := <-r.linkChan:
			if cmd.ty == cmdCreateUploadID {
				r.linkCache.Add(cmd.id, cmd.linkExpiry)
				r.ackChan <- struct{}{}
			}
		}
	}
}
