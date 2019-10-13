package main

import (
	"encoding/json"
	"math/rand"
	"net/http"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// randomAlphanumeric generates a pseudo-random alphanumeric sequence
// of the given length.
func randomAlphanumeric(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

// acceptJSON from the incoming request and respond with error if we're unable to
// decode the response.
func acceptJSON(w http.ResponseWriter, r *http.Request, value interface{}) error {
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(value)
	if err != nil {
		respondError(w, "Invalid JSON in request body.", http.StatusBadRequest)
	}

	return err
}

// respondJSON in this response using the given value.
func respondJSON(w http.ResponseWriter, value interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	err := encoder.Encode(value)
	if err != nil {
		respondError(w, "Error writing JSON data.", http.StatusInternalServerError)
	}

	return err
}

// respondError in the response with the given message and status code.
func respondError(w http.ResponseWriter, msg string, code int) {
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.Encode(ErrorResponse{
		Error: msg,
	})
}
