package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"

	"github.com/adrium/goheif"
	"github.com/disintegration/imaging"
)

// getImage retrieves the byte contents of a specified reader.
//
// The returned byte array should be the image, or an error returnedd if it could not be read.
func getImage(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// convertHeicToJpeg does what it says, and converts HEIC/HEIF images to JPEG so they can be resized.
//
// Returns the converted image, or an error if something went wrong.
func convertHeicToJpeg(reader io.Reader) ([]byte, error) {
	img, err := goheif.Decode(reader)
	if err != nil {
		return nil, err
	}
	buff := new(bytes.Buffer)
	err = jpeg.Encode(buff, img, nil)
	if err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

// resizeImage attempts to resize the supplied image (assuming the bytes represent a jpeg).
//
// Returns a new byte array representing the smaller jpeg.
func resizeImage(origImg []byte) ([]byte, error) {
	imgConf, _, err := image.DecodeConfig(bytes.NewReader(origImg))
	if err != nil {
		return nil, fmt.Errorf("failed to decode byte stream as a jpeg: %w", err)
	}

	if imgConf.Width <= ThumbnailSize && imgConf.Height <= ThumbnailSize {
		return origImg, nil
	}

	original, _, err := image.Decode(bytes.NewReader(origImg))
	if err != nil {
		return nil, fmt.Errorf("failed to decode byte stream as a jpeg: %w", err)
	}

	// a new width/height of zero means "retain aspect ratio", so we only set one
	newWidth := 0
	newHeight := 0
	if imgConf.Width > imgConf.Height {
		newWidth = ThumbnailSize
	} else {
		newHeight = ThumbnailSize
	}

	newImage := imaging.Resize(original, newWidth, newHeight, imaging.Lanczos)

	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, newImage, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encode resized image as jpeg: %w", err)
	}

	return buf.Bytes(), nil
}
