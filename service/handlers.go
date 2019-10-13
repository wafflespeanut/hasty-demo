package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

// registerRoutes for this service in the provided router.
func (service *ImageService) registerRoutes() {
	// We need a router to extract IDs for expiring links and image fetching.
	r := mux.NewRouter()
	amw := AuthMiddleware{
		accessToken: service.accessToken,
	}

	ephemeralEndpoint := fmt.Sprintf("%s/{id}", service.uploadLinkPrefix)
	r.HandleFunc(ephemeralEndpoint, service.handleImageUpload).Methods("POST")
	r.HandleFunc("/images/{id}", func(w http.ResponseWriter, r *http.Request) {
		//
	}).Methods("GET")

	// Endpoints that require an access token are behind the auth middleware.
	s := r.PathPrefix("/admin").Subrouter()
	s.Use(amw.Middleware)

	s.HandleFunc("/ephemeral-links", service.handleLinkCreation).Methods("POST")
	s.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		//
	}).Methods("GET")

	http.Handle("/", r)
}

func (service *ImageService) handleLinkCreation(w http.ResponseWriter, r *http.Request) {
	var req LinkCreationRequest
	err := acceptJSON(w, r, &req)
	if err != nil {
		return
	}

	resp, err := service.CreateUploadLink(req)
	if err != nil {
		respondError(w, err.Error(), http.StatusBadRequest)
	} else {
		respondJSON(w, resp)
	}
}

func (service *ImageService) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uploadID := vars["id"]
	reader, err := r.MultipartReader()
	if err != nil {
		respondError(w, "Expected upload stream to have multipart data", http.StatusBadRequest)
		return
	}

	resp, code := service.StreamImagesToBackend(uploadID, reader)
	if code == streamInvalidUploadID {
		http.Error(w, "Not Found", http.StatusNotFound)
	} else {
		respondJSON(w, resp)
	}
}

// AuthMiddleware for securing some endpoints.
type AuthMiddleware struct {
	accessToken string
}

// Middleware function that's called for endpoints that are gated for authorization.
func (amw *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(headerAccessToken)
		if token == amw.accessToken {
			next.ServeHTTP(w, r)
		} else {
			http.Error(w, "You're not allowed to perform that action.", http.StatusForbidden)
		}
	})
}
