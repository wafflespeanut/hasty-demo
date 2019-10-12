package main

import (
	"github.com/gorilla/mux"
)

// ImageService handles the incoming HTTP requests and proxies the necessary
// data to and from the repository.
type ImageService struct {
	cmdChan chan repoCommand
}

// registerRoutes for this service in the provided router.
func (service *ImageService) registerRoutes(r *mux.Router) {
	//
}
