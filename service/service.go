package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
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
	data             *DataRepository
	objects          *ObjectsRepository
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

	linkID := randomAlphanumeric(uploadLinkIDLength)
	service.data.createUploadID(linkID, expiry)

	return &EphemeralLinkResponse{
		RelativePath: fmt.Sprintf("%s/%s", service.uploadLinkPrefix, linkID),
		Timestamp:    expiry.Format(time.RFC3339),
	}, nil
}

// StreamStatus represents one of the possible status messages for stream processing.
type StreamStatus int

const (
	streamInvalidUploadID = iota
	streamInvalidImage
	streamFailure
	streamSuccess
)

// StreamImagesToBackend validates the given upload ID and streams file chunks from the given
// reader to the repository.
func (service *ImageService) StreamImagesToBackend(linkID string, reader *multipart.Reader) (*ImageUploadResponse, StreamStatus) {
	if service.data.hasExpired(linkID) {
		return nil, streamInvalidUploadID
	}

	response := ImageUploadResponse{
		Processed: []ProcessedImage{},
	}

	buf := make([]byte, defaultBufSize)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}

		ctype := part.Header.Get(headerContentType)
		if ctype == "" || !strings.HasPrefix(ctype, imageMediaType) {
			// Ignore this part if this is not an image. We can bail out
			// with an error in the future, if needed. Or, we could also
			// stream the file and discard it whilst processing. Even then,
			// we can't take it for granted - we have to check the magic
			// numbers when we process the image.
			continue
		}

		hasher := sha256.New()
		imageID := randomAlphanumeric(imageIDLength)
		fileName := part.FileName()

		var totalBytes int
		for {
			n, err := part.Read(buf)
			totalBytes += n

			// Make new slice as we'll update the existing slice in the next read.
			slice := make([]byte, len(buf[:n]))
			copy(slice, buf[:n])
			// FIXME: Right now we're doing this for a file. Should we do this for
			// each part instead? That helps with retrying from the part that was
			// left out in an event of failure.
			hasher.Write(slice)
			service.objects.sendChunk(imageID, slice)

			if err == io.EOF {
				if n != 0 {
					// Send an empty chunk to stop streaming.
					service.objects.sendChunk(imageID, []byte{})
				}

				break
			}
		}

		log.Printf("Processed %s (image ID: %s)\n", fileName, imageID)
		contentHash := fmt.Sprintf("%x", hasher.Sum(nil))

		existingImageID := service.data.fetchIDForHash(contentHash)
		if existingImageID != "" {
			log.Printf("Discarding possible duplicate image (ID: %s)\n", imageID)
			service.objects.discardChunks(imageID)
			imageID = existingImageID
		}

		response.Processed = append(response.Processed, ProcessedImage{
			Filename: fileName,
			ID:       imageID,
			Hash:     contentHash,
			Size:     uint(totalBytes),
		})

		meta := ImageMeta{
			ID:        imageID,
			Hash:      contentHash,
			MediaType: ctype,
			Size:      uint(totalBytes),
			Uploaded:  time.Now().UTC(),
		}

		// Update the repository only if we've encountered a new image.
		if existingImageID == "" {
			// Add known metadata for now.
			service.data.addImageData(meta)
			// ... and queue the image for getting additional data.
			service.objects.queueImageForAnalysis(meta)
		}
	}

	return &response, streamSuccess
}

// StreamImageFromBackend if an image exists for the given image ID.
func (service *ImageService) StreamImageFromBackend(imageID string, h http.Header, w io.Writer) StreamStatus {
	meta := service.data.fetchImageMeta(imageID)
	if meta == nil {
		return streamInvalidImage
	}

	h.Set(headerContentType, meta.MediaType)

	streamChan := service.objects.fetchChunks(imageID)
	for {
		chunk := <-streamChan
		if chunk.err != nil && chunk.err != io.EOF {
			log.Printf("Failed to stream image (ID: %s): %s\n", imageID, chunk.err.Error())
			return streamFailure
		}

		if chunk.isFinal {
			break
		}

		n, err := w.Write(chunk.bytes)
		if n == 0 || err != nil {
			break
		}
	}

	return streamSuccess
}
