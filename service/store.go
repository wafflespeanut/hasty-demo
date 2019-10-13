package main

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

// DataStore is the persistence layer for adding, mutating and querying data.
type DataStore interface {
	addUploadID(id string, expiry time.Time)
	getUploadExpiry(id string)
	addImageData()
}

// ObjectStore is the persistence layer for storing and retrieving objects.
type ObjectStore interface {
	storeChunk(id string, chunk []byte, isFinal bool)
}

// NoOpStore which does nothing.
type NoOpStore struct{}

func (NoOpStore) addImageData()                           {}
func (NoOpStore) addUploadID(id string, expiry time.Time) {}
func (NoOpStore) getUploadExpiry(id string)               {}

// FileStore is used for persisting objects in the system disk.
// Note that this must not be accessed from multiple goroutines.
type FileStore struct {
	// Prefix path for the objects.
	pathPrefix string
	openFds    map[string]*os.File
}

func (store *FileStore) storeChunk(id string, chunk []byte, isFinal bool) {
	// FIXME: When we encounter disk errors, we should probably return
	// 500 status code?

	var err error
	fd, exists := store.openFds[id]
	if !exists {
		log.Printf("Creating new file for image ID: %s\n", id)
		fd, err = os.Create(filepath.Join(store.pathPrefix, id))
		if err != nil {
			log.Printf("Error creating file for image (ID: %s): %s\n", id, err.Error())
			return
		}
	}

	_, err = fd.Write(chunk)
	if err != nil {
		log.Printf("Error writing chunk to file (image ID: %s): %s\n", id, err.Error())
		return
	}

	if isFinal {
		fd.Close()
		delete(store.openFds, id)
	}
}
