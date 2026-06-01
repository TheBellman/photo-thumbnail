package processing

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	thumbnailimage "github.com/TheBellman/photo-thumbnail/internal/image"
	"github.com/TheBellman/photo-thumbnail/internal/storage"
	"github.com/rwcarlsen/goexif/exif"
)

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

// isobmffBox represents a single parsed ISOBMFF box.
type isobmffBox struct {
	boxType string
	// payload is the box content *after* the size+type (and largesize) header.
	payload []byte
}

// canonPRVWuuid as observed in EOS 90D CR3 files.
// Note: differs from some documented values which have 11e4... after the first 6 bytes.
var canonPRVWuuid = []byte{
	0xea, 0xf4, 0x2b, 0x5e,
	0x1c, 0x98, 0x4b, 0x88,
	0xb9, 0xfb,
	0xb7, 0xdc, 0x40, 0x6e, 0x4d, 0x16,
}

// Canon CMT1 uuid: 85c0b687-820f-11e0-8111-f4ce462b6a48
// This box contains raw TIFF/EXIF data; IFD1 holds the small thumbnail.
var canonCMT1uuid = []byte{
	0x85, 0xc0, 0xb6, 0x87,
	0x82, 0x0f, 0x11, 0xe0,
	0x81, 0x11,
	0xf4, 0xce, 0x46, 0x2b, 0x6a, 0x48,
}

// parseBoxAt parses a single box at the given offset within data.
// Returns the box and the offset of the *next* box.
func parseBoxAt(data []byte, offset int) (isobmffBox, int, error) {
	if offset+8 > len(data) {
		return isobmffBox{}, 0, fmt.Errorf("box header truncated at offset %d", offset)
	}

	size := uint64(binary.BigEndian.Uint32(data[offset:]))
	boxType := string(data[offset+4 : offset+8])
	headerLen := 8

	switch size {
	case 1: // 64-bit largesize follows the type field
		if offset+16 > len(data) {
			return isobmffBox{}, 0, fmt.Errorf("largesize box header truncated at offset %d", offset)
		}
		size = binary.BigEndian.Uint64(data[offset+8:])
		headerLen = 16
	case 0: // box extends to end of file
		size = uint64(len(data) - offset)
	}

	end := offset + int(size)
	if end > len(data) || int(size) < headerLen {
		return isobmffBox{}, 0, fmt.Errorf("box '%s' at offset %d has invalid size %d", boxType, offset, size)
	}

	return isobmffBox{
		boxType: boxType,
		payload: data[offset+headerLen : end],
	}, end, nil
}

// findBox scans data at the top level and returns the first box with the given type.
func findBox(data []byte, target string) (isobmffBox, bool) {
	for offset := 0; offset < len(data); {
		b, next, err := parseBoxAt(data, offset)
		if err != nil {
			break
		}
		if b.boxType == target {
			return b, true
		}
		offset = next
	}
	return isobmffBox{}, false
}

// findBoxes scans data and returns ALL boxes with the given type.
func findBoxes(data []byte, target string) []isobmffBox {
	var result []isobmffBox
	for offset := 0; offset < len(data); {
		b, next, err := parseBoxAt(data, offset)
		if err != nil {
			break
		}
		if b.boxType == target {
			result = append(result, b)
		}
		offset = next
	}
	return result
}

// jpegFromPRVW attemps to find the raw Preview image
func jpegFromPRVW(data []byte) ([]byte, error) {
	// The PRVW payload starts with a version+flags header (00 00 00 00),
	// not a "PRVW" fourcc. Rather than parsing the exact header layout,
	// scan for the JPEG SOI marker — more resilient to Canon firmware variations.
	idx := bytes.Index(data, []byte{0xFF, 0xD8, 0xFF})
	if idx < 0 {
		return nil, fmt.Errorf("PRVW: no JPEG SOI marker found in %d bytes", len(data))
	}
	jpeg := data[idx:]
	if err := validateJPEG(jpeg); err != nil {
		return nil, fmt.Errorf("PRVW: %w", err)
	}
	return jpeg, nil
}

// jpegFromCMT1 feeds the raw TIFF/EXIF block inside a CMT1 box to goexif
// and extracts the IFD1 thumbnail.
func jpegFromCMT1(data []byte) ([]byte, error) {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("CMT1: failed to decode EXIF: %w", err)
	}
	thumb, err := x.JpegThumbnail()
	if err != nil {
		return nil, fmt.Errorf("CMT1: no IFD1 thumbnail: %w", err)
	}
	if len(thumb) == 0 {
		return nil, fmt.Errorf("CMT1: thumbnail is empty")
	}
	return thumb, nil
}

func validateJPEG(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("data too short to be a JPEG")
	}
	// SOI marker
	if data[0] != 0xFF || data[1] != 0xD8 {
		return fmt.Errorf("missing JPEG SOI marker (got %02X %02X)", data[0], data[1])
	}
	// EOI marker
	if data[len(data)-2] != 0xFF || data[len(data)-1] != 0xD9 {
		return fmt.Errorf("missing JPEG EOI marker")
	}
	return nil
}
