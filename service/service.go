package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"time"

	"github.com/rickb777/date/period"
)

var (
	errInvalidExpiryTime = errors.New("Invalid expiry time for upload link")
)

// ImageService handles the incoming HTTP requests and proxies the necessary
// data to and from the repository.
type ImageService struct {
	accessToken string

	// FIXME: Add sanitation. Right now,t we assume that this prefix must begin
	// with "/" and must not end with "/".

	// uploadLinkPrefix for ephemeral image upload links.
	uploadLinkPrefix string
	repository       *ImageRepository
}

// CreateUploadLink validates the given request, creates an upload link and returns
// the corresponding response object. Returns error on validation failure.
func (service *ImageService) CreateUploadLink(req LinkCreationRequest) (*EphemeralLinkResponse, error) {
	now := time.Now().UTC()
	expiry := now
	var err error

	// Try to parse duration.
	if req.Duration != "" {
		period, e := period.Parse(req.Duration)
		if e != nil {
			err = e
		} else {
			expiry, _ = period.AddTo(now)
		}
	}

	// Reset error and try to parse exact timestamp.
	if req.Timestamp != "" && ((req.Duration != "" && err != nil) || req.Duration == "") {
		expiry, err = time.Parse(time.RFC3339, req.Timestamp)
	}

	// If we still have an error, bail out.
	if err != nil {
		return nil, errInvalidExpiryTime
	}

	diff := expiry.Sub(now)
	if diff.Seconds() < minExpirySeconds {
		return nil, errInvalidExpiryTime
	}

	linkID := randomAlphanumeric(64)
	service.repository.createUploadID(linkID, expiry)

	return &EphemeralLinkResponse{
		RelativePath: fmt.Sprintf("%s/%s", service.uploadLinkPrefix, linkID),
		Timestamp:    expiry.Format(time.RFC3339),
	}, nil
}

const (
	streamInvalidUploadID = iota
	streamSuccess
)

// StreamImagesToBackend validates the given upload ID and streams file chunks from the given
// reader to the repository.
func (service *ImageService) StreamImagesToBackend(linkID string, reader *multipart.Reader) (*ImageUploadResponse, int) {
	if service.repository.hasExpired(linkID) {
		return nil, streamInvalidUploadID
	}

	response := ImageUploadResponse{
		Processed: []ProcessedImage{},
	}

	buf := make([]byte, 256)
	for {
		hasher := sha256.New()
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}

		ctype := part.Header.Get("Content-Type")
		if ctype == "" || !strings.HasPrefix(ctype, "image/") {
			// We don't know what kind of image we're looking at.
			continue
		}

		imageID := randomAlphanumeric(64)
		var n int
		for {
			n, err = part.Read(buf)
			slice := make([]byte, len(buf[:n]))
			copy(slice, buf[:n])
			// FIXME: Should we do this for each part instead? (That's what AWS does for S3).
			hasher.Write(slice)
			service.repository.sendChunk(imageID, slice)

			if err == io.EOF && n == 0 {
				break
			}
		}

		response.Processed = append(response.Processed, ProcessedImage{
			ID:   imageID,
			Hash: fmt.Sprintf("%x", hasher.Sum(nil)),
		})
	}

	return &response, streamSuccess
}
