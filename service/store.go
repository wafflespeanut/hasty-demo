package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DataStore is the persistence layer for adding, mutating and querying data.
type DataStore interface {
	addUploadID(id string, expiry time.Time)
	getUploadExpiry(id string)
}

// ObjectStore is the persistence layer for storing and retrieving objects.
type ObjectStore interface {
	storeChunk(id string, chunk []byte, isFinal bool)
	retrieveChunks(id string, stream chan<- Chunk)
}

// NoOpStore which does nothing.
type NoOpStore struct{}

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

		store.openFds[id] = fd
	}

	_, err = fd.Write(chunk)
	if err != nil {
		log.Printf("Error writing chunk to file (image ID: %s): %s\n", id, err.Error())
	}

	if isFinal || err != nil {
		fd.Close()
		delete(store.openFds, id)
	}
}

func (store *FileStore) retrieveChunks(id string, stream chan<- Chunk) {
	fd, err := os.Open(filepath.Join(store.pathPrefix, id))
	defer fd.Close()

	if err != nil {
		stream <- Chunk{
			bytes:   []byte{},
			isFinal: true,
			err:     err,
		}

		return
	}

	buf := make([]byte, 256)
	for {
		n, err := fd.Read(buf)
		slice := make([]byte, len(buf[:n]))
		copy(slice, buf[:n])

		stream <- Chunk{
			bytes:   slice,
			isFinal: n == 0,
			err:     err,
		}

		if err == io.EOF {
			if n != 0 {
				// Send final chunk to end stream.
				stream <- Chunk{
					bytes:   []byte{},
					isFinal: n == 0,
					err:     nil,
				}
			}

			break
		}
	}
}
