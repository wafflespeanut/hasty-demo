package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
)

const (
	envAccessToken = "ACCESS_TOKEN"
	envStorePath   = "STORE_PATH"

	defaultPort              = 3000
	defaultLinkCacheCapacity = 1000
	defaultMetaCacheCapacity = 250
	defaultStorePath         = "./store"
)

func main() {
	// Seed PRNG as we use it for generating upload links.
	rand.Seed(time.Now().UnixNano())

	portPtr := flag.Uint("port", defaultPort, "Listening port")
	linksCacheCapPtr := flag.Uint("cache-links", defaultLinkCacheCapacity, "Cache capacity for upload links")
	metaCacheCapPtr := flag.Uint("cache-meta", defaultLinkCacheCapacity, "Cache capacity for image metadata")

	portStr := os.Getenv(envAccessToken)
	if portStr == "" {
		fmt.Printf("Please set %s in the environment for securing endpoints.\n", envAccessToken)
		os.Exit(1)
	}

	// We need a router to extract IDs for expiring links and image fetching.
	router := mux.NewRouter()
	repository, err := initializeRepository(int(*linksCacheCapPtr), int(*metaCacheCapPtr))
	if err != nil {
		fmt.Printf("Error initializing repository: %s", err.Error())
		os.Exit(1)
	}

	go repository.handleCommands()

	service := &ImageService{
		cmdChan: repository.cmdChan,
	}
	service.registerRoutes(router)

	log.Printf("Listening on port %d\n", *portPtr)
	http.ListenAndServe(fmt.Sprintf(":%d", *portPtr), nil)
}
