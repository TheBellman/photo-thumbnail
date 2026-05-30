package processing

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBellman/photo-thumbnail/internal/storage"
)

const thumbnailSize = 200

func TestIsOrf(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "cr3",
			key:  "test.CR3",
			want: false,
		},
		{
			name: "orf",
			key:  "test.ORF",
			want: true,
		},
		{
			name: "jpeg",
			key:  "test.jpeg",
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isORF(testFile(tt.key))
			if result != tt.want {
				t.Fatalf("isCR3() = %t, want %t", result, tt.want)
			}
		})
	}
}

func TestExtractOrfThumbnail(t *testing.T) {
	t.Parallel()
	result, err := extractORFThumbnail(testFile("test.ORF"))
	if err != nil {
		t.Fatalf("extractORFThumbnail() error: %v", err)
	}
	if len(result) == 0 {
		t.Fatalf("extractORFThumbnail() = empty")
	}
}

func TestIsCr3(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "cr3",
			key:  "test.CR3",
			want: true,
		},
		{
			name: "orf",
			key:  "test.ORF",
			want: false,
		},
		{
			name: "jpeg",
			key:  "test.jpeg",
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isCR3(testFile(tt.key))
			if result != tt.want {
				t.Fatalf("isCR3() = %t, want %t", result, tt.want)
			}
		})
	}
}

func TestReadImageBytes(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		contentType string
	}{
		{
			name:        "cr3",
			key:         "test.CR3",
			contentType: "image/x-canon-cr3",
		},
		{
			name:        "orf",
			key:         "test.ORF",
			contentType: "image/x-olympus-orf",
		},
		{
			name:        "jpeg",
			key:         "test.jpeg",
			contentType: "image/jpeg",
		},
		{
			name:        "heic",
			key:         "test.HEIC",
			contentType: "image/heic",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := readImageBytes(mustOpenFile(tt.key), tt.contentType)
			if err != nil {
				t.Fatalf("readImageBytes() error: %v", err)
			}
			if len(result) == 0 {
				t.Fatalf("readImageBytes() = empty")
			}
			if result == nil {
				t.Fatalf("readImageBytes() = returned nil")
			}
		})
	}
}

func TestCreateThumbnailRoutesRawFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr string
	}{
		{
			name:    "routes cr3",
			key:     "photos/test.CR3",
			wantErr: "CR3 image processing is not implemented",
		},
		{
			name:    "routes orf",
			key:     "photos/test.ORF",
			wantErr: "ORF image processing is not implemented",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := CreateThumbnail(tt.key, strings.NewReader("raw image bytes"), "", thumbnailSize)
			if err == nil {
				t.Fatalf("expected error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("CreateThumbnail() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestProcessStandardImage(t *testing.T) {
	t.Parallel()

	orig := makeProcessorTestJPEGBytes(t, thumbnailSize*2, thumbnailSize)
	thumb, err := processStandardImage(bytes.NewReader(orig), storage.JPEG, thumbnailSize)
	if err != nil {
		t.Fatalf("processStandardImage() error: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != thumbnailSize {
		t.Fatalf("expected width %d, got %d", thumbnailSize, bounds.Dx())
	}
	if bounds.Dy() > thumbnailSize {
		t.Fatalf("expected height <= %d, got %d", thumbnailSize, bounds.Dy())
	}
}

func TestProcessStandardImageReadError(t *testing.T) {
	t.Parallel()

	_, err := processStandardImage(errReader{}, storage.JPEG, thumbnailSize)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestReadImageBytesJPEG(t *testing.T) {
	t.Parallel()

	input := []byte{1, 2, 3, 4}
	got, err := readImageBytes(bytes.NewReader(input), storage.JPEG)
	if err != nil {
		t.Fatalf("readImageBytes() error: %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("readImageBytes() = %v, want %v", got, input)
	}
}

func TestReadImageBytesReadError(t *testing.T) {
	t.Parallel()

	_, err := readImageBytes(errReader{}, storage.JPEG)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestPatchORFHeader(t *testing.T) {
	orfData := testFile("test.ORF") // real ORF bytes, expected to start with IIRO

	// Build a synthetic ORF header + dummy payload for cases that don't need a real file.
	makeORF := func(b0, b1, b2, b3 byte) []byte {
		buf := make([]byte, 16)
		buf[0], buf[1], buf[2], buf[3] = b0, b1, b2, b3
		return buf
	}

	tests := []struct {
		name        string
		input       []byte
		wantBytes   []byte // expected first 4 bytes after patch; nil means check not applied
		wantPatched bool   // true if bytes 2-3 should have changed
	}{
		{
			name:        "real ORF file gets header patched",
			input:       orfData,
			wantBytes:   []byte{0x49, 0x49, 0x2A, 0x00},
			wantPatched: true,
		},
		{
			name:        "synthetic IIRO header gets patched",
			input:       makeORF(0x49, 0x49, 0x52, 0x4F),
			wantBytes:   []byte{0x49, 0x49, 0x2A, 0x00},
			wantPatched: true,
		},
		{
			name:        "standard little-endian TIFF passthrough",
			input:       makeORF(0x49, 0x49, 0x2A, 0x00),
			wantBytes:   []byte{0x49, 0x49, 0x2A, 0x00},
			wantPatched: false,
		},
		{
			name:        "big-endian TIFF passthrough",
			input:       makeORF(0x4D, 0x4D, 0x00, 0x2A),
			wantBytes:   []byte{0x4D, 0x4D, 0x00, 0x2A},
			wantPatched: false,
		},
		{
			name:        "unrecognised header passthrough",
			input:       makeORF(0xDE, 0xAD, 0xBE, 0xEF),
			wantBytes:   []byte{0xDE, 0xAD, 0xBE, 0xEF},
			wantPatched: false,
		},
		{
			name:        "too short returns input unchanged",
			input:       []byte{0x49, 0x49},
			wantBytes:   []byte{0x49, 0x49},
			wantPatched: false,
		},
		{
			name:        "nil input returns nil",
			input:       nil,
			wantBytes:   nil,
			wantPatched: false,
		},
		{
			name:        "original slice is not mutated",
			input:       makeORF(0x49, 0x49, 0x52, 0x4F),
			wantBytes:   []byte{0x49, 0x49, 0x2A, 0x00},
			wantPatched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture original header bytes before calling, to detect mutation.
			var originalHeader []byte
			if len(tt.input) >= 4 {
				originalHeader = []byte{tt.input[0], tt.input[1], tt.input[2], tt.input[3]}
			}

			result := patchORFHeader(tt.input)

			// Check expected first 4 bytes.
			if tt.wantBytes != nil {
				if len(result) < len(tt.wantBytes) {
					t.Fatalf("result too short: got %d bytes, want at least %d", len(result), len(tt.wantBytes))
				}
				if !bytes.Equal(result[:len(tt.wantBytes)], tt.wantBytes) {
					t.Errorf("header bytes: got % X, want % X", result[:len(tt.wantBytes)], tt.wantBytes)
				}
			}

			// Check that the output length matches the input length.
			if len(result) != len(tt.input) {
				t.Errorf("length changed: got %d, want %d", len(result), len(tt.input))
			}

			// Check the original slice was not mutated.
			if originalHeader != nil && !bytes.Equal(tt.input[:4], originalHeader) {
				t.Errorf("input slice was mutated: original % X, now % X", originalHeader, tt.input[:4])
			}

			// When a patch is expected, result must be a distinct allocation.
			if tt.wantPatched && len(tt.input) >= 4 {
				if &result[0] == &tt.input[0] {
					t.Error("expected a new allocation for patched result, but got same backing array")
				}
			}

			// When no patch is needed, result should be the same slice (no alloc).
			if !tt.wantPatched && tt.input != nil && len(tt.input) >= 4 {
				if &result[0] != &tt.input[0] {
					t.Error("expected passthrough (same backing array) for unmodified input, but got a copy")
				}
			}
		})
	}
}

func makeProcessorTestJPEGBytes(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	return buf.Bytes()
}

type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	return 0, errors.New("read error")
}

// helper method to read a target file in testdata folder and return as bytes
func testFile(name string) []byte {
	data, err := io.ReadAll(mustOpenFile(name))
	if err != nil {
		log.Fatalf("Failed to read %s", name)
	}
	return data
}

// Updated helper for tests that expect the file to definitely exist
func mustOpenFile(name string) io.ReadCloser {
	f, err := testFileReader(name)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", name, err)
	}
	return f
}

func testFileReader(name string) (io.ReadCloser, error) {
	path := filepath.Join("..", "..", "testdata", name)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}
