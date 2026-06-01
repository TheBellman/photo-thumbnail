package image

import (
	"bytes"
	"errors"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"
)

const thumbnailSize = 200

type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	return 0, errors.New("read error")
}

func makeJPEGBytes(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
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

func TestRead(t *testing.T) {
	t.Parallel()

	input := []byte{1, 2, 3, 4}
	got, err := Read(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Read output mismatch: got %v, want %v", got, input)
	}
}

func TestReadError(t *testing.T) {
	t.Parallel()

	_, err := Read(errReader{})
	if err == nil {
		t.Fatalf("expected error from Read")
	}
}

func TestResizeNoOp(t *testing.T) {
	t.Parallel()

	orig := makeJPEGBytes(t, thumbnailSize, thumbnailSize)
	resized, err := Resize(orig, thumbnailSize)
	if err != nil {
		t.Fatalf("Resize error: %v", err)
	}
	if !bytes.Equal(resized, orig) {
		t.Fatalf("expected Resize to return original bytes for small image")
	}
}

func TestResizeShrinks(t *testing.T) {
	t.Parallel()

	orig := makeJPEGBytes(t, thumbnailSize*2, thumbnailSize)
	resized, err := Resize(orig, thumbnailSize)
	if err != nil {
		t.Fatalf("Resize error: %v", err)
	}

	img, _, err := stdimage.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatalf("decode resized image: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != thumbnailSize {
		t.Fatalf("expected width %d, got %d", thumbnailSize, bounds.Dx())
	}
	if bounds.Dy() > thumbnailSize {
		t.Fatalf("expected height <= %d, got %d", thumbnailSize, bounds.Dy())
	}
}

var _ io.Reader = errReader{}
