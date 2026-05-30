package processing

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	thumbnailimage "github.com/TheBellman/photo-thumbnail/internal/image"
	"github.com/TheBellman/photo-thumbnail/internal/storage"
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

	return nil, fmt.Errorf("CR3 image processing is not implemented")
}

func processORFImage(imgReader io.Reader) ([]byte, error) {
	data, err := thumbnailimage.Read(imgReader)
	if err != nil {
		return nil, fmt.Errorf("read ORF image bytes: %w", err)
	}
	if !isORF(data) {
		return nil, fmt.Errorf("invalid ORF image")
	}

	return nil, fmt.Errorf("ORF image processing is not implemented")
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

// isCR3 checks for the ISOBMFF ftyp box with Canon's 'crx ' brand.
// CR3 byte layout: [size:4][ftyp:4][brand:4]...
func isCR3(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	if string(data[4:8]) != "ftyp" {
		return false
	}
	brand := string(data[8:12])
	return brand == "crx " || brand == "CRX "
}

// isORF checks for the Olympus Raw Format TIFF magic bytes.
// Little-endian ORF: "IIRO"  (49 49 52 4F)
// Big-endian    ORF: "MMOR"  (4D 4D 4F 52)
func isORF(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	sig := string(data[0:4])
	return sig == "IIRO" || sig == "MMOR"
}

func readImageBytes(imgReader io.Reader, contentType string) ([]byte, error) {
	if contentType == storage.HEIC {
		imageBytes, err := thumbnailimage.ConvertHEICToJPEG(imgReader)
		if err != nil {
			return nil, fmt.Errorf("convert HEIC to JPEG: %w", err)
		}

		return imageBytes, nil
	}

	imageBytes, err := thumbnailimage.Read(imgReader)
	if err != nil {
		return nil, fmt.Errorf("read image bytes: %w", err)
	}

	return imageBytes, nil
}
