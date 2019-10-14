package main

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DataStore is the persistence layer for adding, mutating and querying data.
type DataStore interface {
	// initialize this store.
	initialize() error
	// addUploadID with the given expiry timestamp.
	addUploadID(id string, expiry time.Time) error
	// getUploadExpiry for the given upload ID.
	getUploadExpiry(id string) (*time.Time, error)
	// addImageMeta to this store.
	addImageMeta(meta ImageMeta) error
	// fetchImageMeta for the given image ID.
	fetchImageMeta(id string) (*ImageMeta, error)
	// fetchMetaForHash of some image.
	fetchMetaForHash(hash string) (*ImageMeta, error)
	// updateImageMeta existing for some image.
	updateImageMeta(meta ImageMeta) error
	// getServiceStats for the data we have collected so far.
	getServiceStats() (*ServiceStats, error)
}

// ObjectStore is the persistence layer for storing and retrieving objects.
type ObjectStore interface {
	// storeChunk for the given image ID.
	storeChunk(id string, chunk []byte, isFinal bool)
	// retrieveChunks for the given image ID and send it through the given channel.
	retrieveChunks(id string, stream chan<- Chunk)
	// discardObject corresponding to the given image ID.
	discardObject(id string)
	// getImageReader corresponding to the given image ID.
	getImageReader(id string) (io.Reader, error)
	// cleanupImageReader for the given ID and reader obtained using `getImageReader`
	cleanupImageReader(id string, reader io.Reader) error
}

// NoOpStore which does nothing.
type NoOpStore struct{}

func (NoOpStore) initialize() error                             { return nil }
func (NoOpStore) addUploadID(id string, expiry time.Time) error { return nil }
func (NoOpStore) addImageMeta(meta ImageMeta) error             { return nil }
func (NoOpStore) updateImageMeta(meta ImageMeta) error          { return nil }
func (NoOpStore) getUploadExpiry(id string) (*time.Time, error) {
	return nil, errors.New("no-op")
}
func (NoOpStore) fetchMetaForHash(hash string) (*ImageMeta, error) {
	return nil, errors.New("no-op")
}
func (NoOpStore) fetchImageMeta(id string) (*ImageMeta, error) {
	return nil, errors.New("no-op")
}
func (NoOpStore) getServiceStats() (*ServiceStats, error) {
	return nil, errors.New("no-op")
}

// MARK: File store.

// FileStore is used for persisting objects in the system disk.
// Note that this must not be accessed from multiple goroutines.
type FileStore struct {
	// Prefix path for the objects.
	pathPrefix string
	openFds    map[string]*os.File
}

// MARK: `DataStore` interface methods.

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

	buf := make([]byte, defaultBufSize)
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

func (store *FileStore) discardObject(id string) {
	os.Remove(filepath.Join(store.pathPrefix, id))
}

func (store *FileStore) getImageReader(id string) (io.Reader, error) {
	return os.Open(filepath.Join(store.pathPrefix, id))
}

func (store *FileStore) cleanupImageReader(id string, reader io.Reader) error {
	fd, ok := reader.(*os.File)
	if ok {
		fd.Close()
	}

	return nil
}
