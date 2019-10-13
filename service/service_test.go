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
	id := service.repository.linkCache.Keys()[0]
	assert.EqualValues(fmt.Sprintf("/booya/%s", id), req.RelativePath)
	expiry, _ := service.repository.linkCache.Get(id)
	diff := expiry.(time.Time).Sub(reqExpiry)
	assert.Zero(int(diff.Seconds()))

	req, err = service.CreateUploadLink(LinkCreationRequest{
		Duration:  "P2DT3H",
		Timestamp: "invalid timestamp",
	})

	assert.Nil(err)
	id = service.repository.linkCache.Keys()[1]
	assert.EqualValues(fmt.Sprintf("/booya/%s", id), req.RelativePath)
	expiry, _ = service.repository.linkCache.Get(id)
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
	repo := &ImageRepository{
		linkCache: lCache,
		metaCache: mCache,
		dataStore: NoOpStore{},
		objectStore: &FileStore{
			pathPrefix: "./",
			openFds:    make(map[string]*os.File),
		},
		cmdHub: MessageHub{
			cmdChan:  make(chan repoMessage),
			respChan: make(chan interface{}),
			ackChan:  make(chan struct{}),
		},
		streamHub: MessageHub{
			cmdChan:  make(chan repoMessage),
			respChan: make(chan interface{}),
			ackChan:  make(chan struct{}),
		},
	}

	// It's fine if all those goroutines keep blocking - they're efficient,
	// and they'll be killed when the program ends.
	go repo.handleCommands()

	return &ImageService{
		accessToken:      "foobar",
		uploadLinkPrefix: "/booya",
		repository:       repo,
	}
}
