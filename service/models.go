package main

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
	ID   string `json:"id"`
	Hash string `json:"hash"`
}

// ImageUploadResponse after uploading one or more images.
type ImageUploadResponse struct {
	Processed []ProcessedImage `json:"processed"`
}
