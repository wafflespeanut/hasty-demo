package main

import "time"

// LinkCreationRequest for creating ephemeral links.
type LinkCreationRequest struct {
	// Duration (from now) in ISO 8601 duration format.
	Duration string `json:"sinceNow"`
	// Timestamp in ISO 8601 (RFC 3339) format.
	Timestamp string `json:"timeExact"`
}

// EphemeralLinkResponse for generated ephemeral links.
type EphemeralLinkResponse struct {
	// RelativePath to the newly generated ephemeral link.
	RelativePath string `json:"relativePath"`
	// Timestamp after which this link expires.
	Timestamp string `json:"expiresOn"`
}

// ErrorResponse for the API.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ProcessedImage from an upload.
type ProcessedImage struct {
	Filename string `json:"name"`
	ID       string `json:"id"`
	Hash     string `json:"hash"`
	Size     uint   `json:"size"`
}

// ImageUploadResponse after uploading one or more images.
type ImageUploadResponse struct {
	Processed []ProcessedImage `json:"processed"`
}

// ImageMeta for holding metadata for images.
type ImageMeta struct {
	ID          string    `json:"id"`
	Hash        string    `json:"hash"`
	MediaType   string    `json:"mediaType"`
	Size        uint      `json:"size"`
	Uploaded    time.Time `json:"uploadedOn"`
	CameraModel string    `json:"cameraModel,omitempty"`
}

// applyDefaults for unknown metadata.
func (m *ImageMeta) applyDefaults() {
	m.CameraModel = "unknown"
}
