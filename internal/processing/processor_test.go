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

	want := testFile("test_orf_thumb.jpg")

	if !bytes.Equal(result, want) {
		t.Errorf("extractORFThumbnail() = %d bytes, want %d bytes", len(result), len(want))
		// Help narrow down where the mismatch is.
		if len(result) == len(want) {
			for i := range result {
				if result[i] != want[i] {
					t.Errorf("first difference at byte %d: got 0x%02X, want 0x%02X", i, result[i], want[i])
					break
				}
			}
		}
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
