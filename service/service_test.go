package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/stretchr/testify/assert"
)

func TestLinkCreation(t *testing.T) {
	assert := assert.New(t)
	service := createService()
	now := time.Now()
	reqExpiry := now.Add(time.Duration(60) * time.Second)

	req, err := service.CreateUploadLink(LinkCreationRequest{
		Duration:  "some invalid string",
		Timestamp: reqExpiry.Format(time.RFC3339),
	})

	assert.Nil(err)
	id := service.data.linkCache.Keys()[0]
	assert.EqualValues(fmt.Sprintf("/booya/%s", id), req.RelativePath)
	expiry, _ := service.data.linkCache.Get(id)
	diff := expiry.(time.Time).Sub(reqExpiry)
	assert.Zero(int(diff.Seconds()))

	req, err = service.CreateUploadLink(LinkCreationRequest{
		Duration:  "P2DT3H",
		Timestamp: "invalid timestamp",
	})

	assert.Nil(err)
	id = service.data.linkCache.Keys()[1]
	assert.EqualValues(fmt.Sprintf("/booya/%s", id), req.RelativePath)
	expiry, _ = service.data.linkCache.Get(id)
	diff = expiry.(time.Time).Sub(now)
	assert.EqualValues(2*86400+3*3600, int(diff.Seconds()))
}

func TestInvalidTimestamp(t *testing.T) {
	assert := assert.New(t)
	service := createService()
	now := time.Now()
	reqExpiry := now.Add(time.Duration(10) * time.Second) // 10 seconds in the future is invalid.

	req, err := service.CreateUploadLink(LinkCreationRequest{
		Duration:  "some invalid string",
		Timestamp: "invalid timestamp",
	})
	assert.Nil(req)
	assert.EqualValues(err, errInvalidExpiryTime)

	req, err = service.CreateUploadLink(LinkCreationRequest{
		Duration:  "some invalid string",
		Timestamp: reqExpiry.Format(time.RFC3339),
	})
	assert.Nil(req)
	assert.EqualValues(err, errInvalidExpiryTime)
}

func createService() *ImageService {
	lCache, _ := lru.New(defaultLinkCacheCapacity)
	mCache, _ := lru.New(defaultMetaCacheCapacity)
	hashes, _ := lru.New(defaultHashesCacheCapacity)

	dataRepo := &DataRepository{
		linkCache: lCache,
		metaCache: mCache,
		hashes:    hashes,
		dataStore: NoOpStore{},
		cmdHub:    NewMessageHub(),
	}

	// It's fine if all those goroutines keep blocking - they're efficient,
	// and they'll be killed when the program ends.
	go dataRepo.handleCommands()

	return &ImageService{
		accessToken:      "foobar",
		uploadLinkPrefix: "/booya",
		data:             dataRepo,
		objects: &ObjectsRepository{
			data: dataRepo,
			objectStore: &FileStore{
				pathPrefix: "./",
				openFds:    make(map[string]*os.File),
			},
			streamHub: NewMessageHub(),
			imageHub:  NewMessageHub(),
		},
	}
}
