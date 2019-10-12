package main

// DataStore is the persistence layer for adding, mutating and querying data.
type DataStore interface {
	addImageData()
}

// ObjectStore is the persistence layer for storing and retrieving objects.
type ObjectStore interface {
	storeImage()
	retrieveImage()
}

// NoOpStore which does nothing.
type NoOpStore struct{}

func (NoOpStore) addImageData() {}

// FileStore is used for persisting objects in the system disk.
type FileStore struct {
	// Prefix path for the objects.
	path string
}

func (store FileStore) storeImage() {}

func (store FileStore) retrieveImage() {}
