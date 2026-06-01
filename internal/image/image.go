package image

import (
	"bytes"
	"fmt"
	stdimage "image"
	"image/jpeg"
	"io"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/heic"
)

// Read retrieves the byte contents of a specified reader.
func Read(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ConvertHEICToJPEG converts HEIC/HEIF images to JPEG so they can be resized.
func ConvertHEICToJPEG(reader io.Reader) ([]byte, error) {
	img, err := heic.Decode(reader)
	if err != nil {
		return nil, err
	}

	buff := new(bytes.Buffer)
	err = jpeg.Encode(buff, img, &jpeg.Options{Quality: 80})
	if err != nil {
		return nil, err
	}

	return buff.Bytes(), nil
}

// Resize attempts to resize the supplied image.
func Resize(origImg []byte, thumbnailSize int) ([]byte, error) {
	imgConf, _, err := stdimage.DecodeConfig(bytes.NewReader(origImg))
	if err != nil {
		return nil, fmt.Errorf("failed to decode byte stream as a jpeg: %w", err)
	}

	if imgConf.Width <= thumbnailSize && imgConf.Height <= thumbnailSize {
		return origImg, nil
	}

	original, _, err := stdimage.Decode(bytes.NewReader(origImg))
	if err != nil {
		return nil, fmt.Errorf("failed to decode byte stream as a jpeg: %w", err)
	}

	newWidth := 0
	newHeight := 0
	if imgConf.Width > imgConf.Height {
		newWidth = thumbnailSize
	} else {
		newHeight = thumbnailSize
	}

	newImage := imaging.Resize(original, newWidth, newHeight, imaging.Lanczos)

	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, newImage, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encode resized image as jpeg: %w", err)
	}

	return buf.Bytes(), nil
}
