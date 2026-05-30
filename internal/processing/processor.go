package processing

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"path"
	"strings"

	thumbnailimage "github.com/TheBellman/photo-thumbnail/internal/image"
	"github.com/TheBellman/photo-thumbnail/internal/storage"
	"github.com/rwcarlsen/goexif/exif"
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
	// ORF files use the magic bytes 'IIRO' (49 49 52 4F) rather than the
	// standard little-endian TIFF marker 'II*\x00' (49 49 2A 00).
	// The goexif library rejects anything that doesn't match the TIFF spec,
	// so we patch a copy of the header before decoding.
	patched := patchORFHeader(data)

	debugORFTags(data)

	x, err := exif.Decode(bytes.NewReader(patched))
	if err != nil {
		return nil, fmt.Errorf("ORF: failed to decode EXIF: %w", err)
	}

	// Olympus stores the thumbnail as a raw blob in ThumbnailImage (0x0501)
	// rather than via the standard JPEGInterchangeFormat pointer in IFD1,
	// so JpegThumbnail() won't find it.
	tag, err := x.Get("ThumbnailImage")
	if err != nil {
		return nil, fmt.Errorf("ORF: no thumbnail tag: %w", err)
	}

	// The tag value is the raw JPEG bytes — strip the enclosing quotes that
	// tiff.Tag.String() would add; Val gives us the underlying []byte directly.
	thumb := tag.Val
	if len(thumb) == 0 {
		return nil, fmt.Errorf("ORF: thumbnail tag is empty")
	}

	// Sanity check: verify it's actually a JPEG (SOI marker FF D8).
	if len(thumb) < 2 || thumb[0] != 0xFF || thumb[1] != 0xD8 {
		return nil, fmt.Errorf("ORF: thumbnail does not look like a JPEG (got % X)", thumb[:min(4, len(thumb))])
	}

	return thumb, nil
}

func debugORFTags(data []byte) {
	patched := patchORFHeader(data)
	x, err := exif.Decode(bytes.NewReader(patched))
	if err != nil {
		log.Fatalf("decode: %v", err)
	}

	// MarshalJSON encodes all decoded tags — just print it raw
	b, err := x.MarshalJSON()
	if err != nil {
		log.Fatalf("json: %v", err)
	}
	fmt.Println(string(b))
}

// patchORFHeader returns a shallow copy of data with the ORF-specific TIFF
// magic bytes replaced by the standard little-endian TIFF marker.
// It only copies the first 4 bytes; the rest is shared to avoid allocating
// a full duplicate of potentially large raw files.
func patchORFHeader(data []byte) []byte {
	if len(data) < 4 {
		return data
	}
	// Confirm this actually looks like an ORF header before touching it.
	if data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x52 && data[3] == 0x4F {
		patched := make([]byte, len(data))
		copy(patched, data)
		patched[2] = 0x2A // '*'
		patched[3] = 0x00
		return patched
	}
	return data
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
