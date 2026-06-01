package processing

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"path"
	"strings"

	thumbnailimage "github.com/TheBellman/photo-thumbnail/internal/image"
)

// CreateThumbnail generates a thumbnail for the given image based on its file extension and content type.
func CreateThumbnail(key string, imgReader io.Reader, contentType string, thumbnailSize int) ([]byte, error) {
	switch strings.ToLower(path.Ext(key)) {
	case ".cr3":
		return processCR3Image(imgReader)
	case ".orf":
		return processORFImage(imgReader)
	default:
		return processStandardImage(imgReader, contentType, thumbnailSize)
	}
}

func processStandardImage(imgReader io.Reader, contentType string, thumbnailSize int) ([]byte, error) {
	imageBytes, err := readImageBytes(imgReader, contentType)
	if err != nil {
		return nil, err
	}

	thumbBytes, err := thumbnailimage.Resize(imageBytes, thumbnailSize)
	if err != nil {
		return nil, fmt.Errorf("create thumbnail image: %w", err)
	}

	err = validateJPEG(thumbBytes)
	if err != nil {
		return nil, err
	}

	return thumbBytes, nil
}

func processCR3Image(imgReader io.Reader) ([]byte, error) {
	data, err := thumbnailimage.Read(imgReader)
	if err != nil {
		return nil, fmt.Errorf("read CR3 image bytes: %w", err)
	}
	if !isCR3(data) {
		return nil, fmt.Errorf("invalid CR3 image")
	}

	thumbBytes, err := extractCR3Thumbnail(data)

	err = validateJPEG(thumbBytes)
	if err != nil {
		return nil, err
	}

	return thumbBytes, err
}

func processORFImage(imgReader io.Reader) ([]byte, error) {
	data, err := thumbnailimage.Read(imgReader)
	if err != nil {
		return nil, fmt.Errorf("read ORF image bytes: %w", err)
	}
	if !isORF(data) {
		return nil, fmt.Errorf("invalid ORF image")
	}

	thumbBytes, err := extractORFThumbnail(data)

	err = validateJPEG(thumbBytes)
	if err != nil {
		return nil, err
	}

	return thumbBytes, err
}

// extractORFThumbnail extracts the thumbnail from the given ORF image.
func extractORFThumbnail(data []byte) ([]byte, error) {
	// The thumbnail is embedded in the Olympus MakerNote which dsoprea/go-exif
	// does not parse. Instead we scan for the JPEG SOI marker (FF D8 FF) and
	// extract the contiguous JPEG blob directly from the raw ORF bytes.
	soi := []byte{0xFF, 0xD8, 0xFF}
	start := bytes.Index(data, soi)
	if start == -1 {
		return nil, fmt.Errorf("ORF: no JPEG SOI marker found")
	}

	// Find the EOI marker (FF D9) that closes the JPEG.
	eoi := []byte{0xFF, 0xD9}
	end := bytes.Index(data[start:], eoi)
	if end == -1 {
		return nil, fmt.Errorf("ORF: no JPEG EOI marker found")
	}
	// end is relative to start, and we want to include the 2 EOI bytes.
	thumb := data[start : start+end+2]

	return thumb, nil
}

// extractCR3Thumbnail extracts the thumbnail from the given CR3 binary blob.
func extractCR3Thumbnail(data []byte) ([]byte, error) {
	// Search uuid boxes at BOTH the top level and inside moov.
	// On the EOS 90D (and many other Canon ISOBMFF cameras) the PRVW uuid
	// sits at the top level of the file, not nested inside moov.
	var allUUIDs []isobmffBox

	topLevel := findBoxes(data, "uuid")
	allUUIDs = append(allUUIDs, topLevel...)

	if moov, found := findBox(data, "moov"); found {
		allUUIDs = append(allUUIDs, findBoxes(moov.payload, "uuid")...)
	}

	if len(allUUIDs) == 0 {
		return nil, fmt.Errorf("CR3: no uuid boxes found anywhere in file")
	}

	var prvwPayload, cmt1Payload []byte
	for _, ub := range allUUIDs {
		if len(ub.payload) < 16 {
			continue
		}
		switch {
		case bytes.Equal(ub.payload[:16], canonPRVWuuid):
			prvwPayload = ub.payload[16:]
		case bytes.Equal(ub.payload[:16], canonCMT1uuid):
			cmt1Payload = ub.payload[16:]
		}
	}

	if len(prvwPayload) > 0 {
		thumb, err := jpegFromPRVW(prvwPayload)
		if err == nil {
			return thumb, nil
		}
		log.Printf("CR3: PRVW parse failed: %v", err)
	}

	if len(cmt1Payload) > 0 {
		thumb, err := jpegFromCMT1(cmt1Payload)
		if err == nil {
			return thumb, nil
		}
		return nil, fmt.Errorf("CR3: PRVW and CMT1 both failed (CMT1: %w)", err)
	}

	return nil, fmt.Errorf("CR3: neither PRVW nor CMT1 uuid found (checked %d uuid boxes)", len(allUUIDs))
}
