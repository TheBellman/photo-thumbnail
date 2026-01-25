package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"
)

type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	return 0, errors.New("read error")
}

func makeJPEGBytes(t *testing.T, width int, height int) []byte {
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

func TestGetImage(t *testing.T) {
	t.Parallel()

	input := []byte{1, 2, 3, 4}
	got, err := getImage(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("getImage error: %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("getImage output mismatch: got %v, want %v", got, input)
	}
}

func TestGetImageReadError(t *testing.T) {
	t.Parallel()

	_, err := getImage(errReader{})
	if err == nil {
		t.Fatalf("expected error from getImage")
	}
}

func TestResizeImageNoOp(t *testing.T) {
	t.Parallel()

	orig := makeJPEGBytes(t, ThumbnailSize, ThumbnailSize)
	resized, err := resizeImage(orig)
	if err != nil {
		t.Fatalf("resizeImage error: %v", err)
	}
	if !bytes.Equal(resized, orig) {
		t.Fatalf("expected resizeImage to return original bytes for small image")
	}
}

func TestResizeImageShrinks(t *testing.T) {
	t.Parallel()

	orig := makeJPEGBytes(t, ThumbnailSize*2, ThumbnailSize)
	resized, err := resizeImage(orig)
	if err != nil {
		t.Fatalf("resizeImage error: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatalf("decode resized image: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != ThumbnailSize {
		t.Fatalf("expected width %d, got %d", ThumbnailSize, bounds.Dx())
	}
	if bounds.Dy() > ThumbnailSize {
		t.Fatalf("expected height <= %d, got %d", ThumbnailSize, bounds.Dy())
	}
}

var _ io.Reader = errReader{}
