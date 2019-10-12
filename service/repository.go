package main

import (
	"log"
	"os"
	"strings"

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

	cmdChan := make(chan repoCommand)

	return &ImageRepository{
		linkCache,
		metaCache,
		dataStore,
		objectStore,
		cmdChan,
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
	cmdChan     chan repoCommand
}

type repoCmdType int

const (
	cmdCreateLink = iota
	cmdIsValidLink
	cmdAddImage
)

type repoCommand struct {
	ty repoCmdType
}

func (r *ImageRepository) handleCommands() {

}
