package main

import (
	"errors"
	"fmt"
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
