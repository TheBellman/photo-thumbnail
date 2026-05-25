package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	JPEG = "image/jpeg"
	HEIC = "image/heic"
)

// Client helps with mocking access to S3.
type Client interface {
	GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// GetImageReader tries to get an io.Reader exposing the body of an image given the bucket and key. It will fail
// if the provided object is not a supported file type. It returns the reader along with the content type.
func GetImageReader(ctx context.Context, service Client, bucket string, key string) (io.Reader, string, error) {
	result, err := service.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("error fetching from s3: %w", err)
	}

	contentType := ""
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	ext := strings.ToLower(path.Ext(key))
	if contentType == "" {
		switch ext {
		case ".heic":
			contentType = HEIC
		case ".jpg", ".jpeg":
			contentType = JPEG
		}
	}

	if ext == ".cr3" || ext == ".heic" || contentType == HEIC || contentType == JPEG {
		return result.Body, contentType, nil
	}

	return nil, "", fmt.Errorf("only JPEG, CR3 and HEIC supported, fetched file %s was reported as %s",
		key,
		contentType)
}

// SaveThumbnail tries to save the supplied data to the desired bucket and key.
func SaveThumbnail(ctx context.Context, service Client, data []byte, bucket string, key string) error {
	reader := bytes.NewReader(data)
	_, err := service.PutObject(ctx, &s3.PutObjectInput{
		Body:          reader,
		Bucket:        aws.String(bucket),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String(JPEG),
		Key:           aws.String(key),
	})

	return err
}
