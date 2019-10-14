package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

const (
	envAccessToken = "ACCESS_TOKEN"
	envStorePath   = "STORE_PATH"
	envPostgresURL = "POSTGRES_URL"

	defaultBufSize             = 512
	defaultPort                = 3000
	defaultLinkCacheCapacity   = 1000
	defaultMetaCacheCapacity   = 250
	defaultHashesCacheCapacity = 1000
	defaultStorePath           = "./store"
	defaultUploadLinkPrefix    = "/uploads"
	minExpirySeconds           = 30
	uploadLinkIDLength         = 48
	imageIDLength              = 48

	headerAccessToken = "X-Access-Token"
	headerContentType = "Content-Type"
	imageMediaType    = "image/"
)

func main() {
	// Seed PRNG as we use it for generating upload links.
	rand.Seed(time.Now().UnixNano())

	portPtr := flag.Uint("port", defaultPort, "Listening port")
	linksCacheCapPtr := flag.Uint("cache-links", defaultLinkCacheCapacity, "Cache capacity for upload links")
	metaCacheCapPtr := flag.Uint("cache-meta", defaultLinkCacheCapacity, "Cache capacity for image metadata")
	hashesCacheCapPtr := flag.Uint("cache-hashes", defaultHashesCacheCapacity, "Cache capacity for image hashes")

	token := os.Getenv(envAccessToken)
	if token == "" {
		fmt.Printf("Please set %s in the environment for securing endpoints.\n", envAccessToken)
		os.Exit(1)
	}

	dataRepo, err := NewDataRepository(int(*linksCacheCapPtr), int(*metaCacheCapPtr), int(*hashesCacheCapPtr))
	if err != nil {
		fmt.Printf("Error initializing data repository: %s", err.Error())
		os.Exit(1)
	}

	objectsRepo, err := NewObjectsRepository(dataRepo)
	if err != nil {
		fmt.Printf("Error initializing objects repository: %s", err.Error())
		os.Exit(1)
	}

	go dataRepo.handleCommands()   // for processing API commands.
	go objectsRepo.processChunks() // for streaming images back and forth.
	go objectsRepo.processImages() // for processing stored images one by one.

	service := &ImageService{
		accessToken:      token,
		data:             dataRepo,
		objects:          objectsRepo,
		uploadLinkPrefix: defaultUploadLinkPrefix,
	}
	service.registerRoutes()

	log.Printf("Listening on port %d\n", *portPtr)
	http.ListenAndServe(fmt.Sprintf(":%d", *portPtr), nil)
}
