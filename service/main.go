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

	defaultBufSize           = 512
	defaultPort              = 3000
	defaultLinkCacheCapacity = 1000
	defaultMetaCacheCapacity = 250
	defaultStorePath         = "./store"
	defaultUploadLinkPrefix  = "/uploads"
	minExpirySeconds         = 30
	uploadLinkIDLength       = 48
	imageIDLength            = 48

	headerAccessToken = "X-Access-Token"
	headerContentType = "Content-Type"
)

func main() {
	// Seed PRNG as we use it for generating upload links.
	rand.Seed(time.Now().UnixNano())

	portPtr := flag.Uint("port", defaultPort, "Listening port")
	linksCacheCapPtr := flag.Uint("cache-links", defaultLinkCacheCapacity, "Cache capacity for upload links")
	metaCacheCapPtr := flag.Uint("cache-meta", defaultLinkCacheCapacity, "Cache capacity for image metadata")

	token := os.Getenv(envAccessToken)
	if token == "" {
		fmt.Printf("Please set %s in the environment for securing endpoints.\n", envAccessToken)
		os.Exit(1)
	}

	repository, err := initializeRepository(int(*linksCacheCapPtr), int(*metaCacheCapPtr))
	if err != nil {
		fmt.Printf("Error initializing repository: %s", err.Error())
		os.Exit(1)
	}

	go repository.handleCommands()
	go repository.processChunks()
	go repository.processImages()

	service := &ImageService{
		accessToken:      token,
		repository:       repository,
		uploadLinkPrefix: defaultUploadLinkPrefix,
	}
	service.registerRoutes()

	log.Printf("Listening on port %d\n", *portPtr)
	http.ListenAndServe(fmt.Sprintf(":%d", *portPtr), nil)
}
