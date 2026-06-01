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
	"testing"

	"github.com/TheBellman/photo-thumbnail/internal/storage"
)

const thumbnailSize = 200

func TestExtractOrfThumbnail(t *testing.T) {
	t.Parallel()
	result, err := extractORFThumbnail(testFile("test.ORF"))
	if err != nil {
		t.Fatalf("extractORFThumbnail() error: %v", err)
	}

	err = validateJPEG(result)
	if err != nil {
		t.Fatalf("extractCR3Thumbnail() error: %v", err)
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

func TestExtractCR3Thumbnail(t *testing.T) {
	t.Parallel()
	result, err := extractCR3Thumbnail(testFile("test.CR3"))
	if err != nil {
		t.Fatalf("extractORFThumbnail() error: %v", err)
	}

	err = validateJPEG(result)
	if err != nil {
		t.Fatalf("extractCR3Thumbnail() error: %v", err)
	}

	want := testFile("test_crf_thumb.jpg")

	if !bytes.Equal(result, want) {
		t.Errorf("extractCFFThumbnail() = %d bytes, want %d bytes", len(result), len(want))
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

func TestCreateThumbnailRoutesRawFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{
			name: "routes cr3",
			key:  "test.CR3",
		},
		{
			name: "routes orf",
			key:  "test.ORF",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			thumbData, err := CreateThumbnail(tt.key, mustOpenFile(tt.key), "", thumbnailSize)
			if err != nil {
				t.Fatalf("CreateThumbnail() error: %v", err)
			}
			if len(thumbData) == 0 {
				t.Fatalf("CreateThumbnail() = empty")
			}

			err = validateJPEG(thumbData)
			if err != nil {
				t.Fatalf("CreateThumbnail() error: %v", err)
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

// helper method to read a target file in testdata folder and return as bytes - this is used in utils_test.go as well
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
