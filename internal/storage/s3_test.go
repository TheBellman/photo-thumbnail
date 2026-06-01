package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3Client struct {
	getObjectOutput *s3.GetObjectOutput
	getObjectError  error
	getObjectInput  *s3.GetObjectInput

	putObjectOutput *s3.PutObjectOutput
	putObjectError  error
	putObjectInput  *s3.PutObjectInput
}

func (f *fakeS3Client) GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.getObjectInput = input
	if f.getObjectError != nil {
		return nil, f.getObjectError
	}
	return f.getObjectOutput, nil
}

func (f *fakeS3Client) PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putObjectInput = input
	if f.putObjectError != nil {
		return nil, f.putObjectError
	}
	return f.putObjectOutput, nil
}

func TestGetImageReaderSupportedFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		key         string
		contentType *string
		wantType    string
	}{
		{
			name:     "jpeg by extension",
			key:      "photos/test.jpg",
			wantType: JPEG,
		},
		{
			name:     "jpeg uppercase extension",
			key:      "photos/test.JPG",
			wantType: JPEG,
		},
		{
			name:     "heic by extension",
			key:      "photos/test.HEIC",
			wantType: HEIC,
		},
		{
			name:     "cr3 by extension",
			key:      "photos/test.CR3",
			wantType: "",
		},
		{
			name:     "orf by extension",
			key:      "photos/test.ORF",
			wantType: "",
		},
		{
			name:        "jpeg by content type",
			key:         "photos/test",
			contentType: aws.String(JPEG),
			wantType:    JPEG,
		},
		{
			name:        "heic by content type",
			key:         "photos/test",
			contentType: aws.String(HEIC),
			wantType:    HEIC,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := "image data"
			client := &fakeS3Client{
				getObjectOutput: &s3.GetObjectOutput{
					Body:        io.NopCloser(strings.NewReader(body)),
					ContentType: tt.contentType,
				},
			}

			reader, contentType, err := GetImageReader(context.Background(), client, "source-bucket", tt.key)
			if err != nil {
				t.Fatalf("GetImageReader() error: %v", err)
			}
			if contentType != tt.wantType {
				t.Fatalf("GetImageReader() contentType = %q, want %q", contentType, tt.wantType)
			}
			if client.getObjectInput == nil {
				t.Fatalf("expected GetObject input to be captured")
			}
			if got := aws.ToString(client.getObjectInput.Bucket); got != "source-bucket" {
				t.Fatalf("GetObject bucket = %q, want %q", got, "source-bucket")
			}
			if got := aws.ToString(client.getObjectInput.Key); got != tt.key {
				t.Fatalf("GetObject key = %q, want %q", got, tt.key)
			}

			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("read returned reader: %v", err)
			}
			if string(data) != body {
				t.Fatalf("reader body = %q, want %q", string(data), body)
			}
		})
	}
}

func TestGetImageReaderUnsupportedFile(t *testing.T) {
	t.Parallel()

	client := &fakeS3Client{
		getObjectOutput: &s3.GetObjectOutput{
			Body: io.NopCloser(strings.NewReader("not image data")),
		},
	}

	_, _, err := GetImageReader(context.Background(), client, "source-bucket", "photos/test.txt")
	if err == nil {
		t.Fatalf("expected unsupported file error")
	}
	if !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("GetImageReader() error = %q, want unsupported file type", err.Error())
	}
}

func TestGetImageReaderGetObjectError(t *testing.T) {
	t.Parallel()

	client := &fakeS3Client{
		getObjectError: errors.New("s3 unavailable"),
	}

	_, _, err := GetImageReader(context.Background(), client, "source-bucket", "photos/test.jpg")
	if err == nil {
		t.Fatalf("expected GetObject error")
	}
	if !strings.Contains(err.Error(), "error fetching from s3") {
		t.Fatalf("GetImageReader() error = %q, want wrapped s3 error", err.Error())
	}
}

func TestSaveThumbnail(t *testing.T) {
	t.Parallel()

	client := &fakeS3Client{
		putObjectOutput: &s3.PutObjectOutput{},
	}

	data := []byte{1, 2, 3, 4}
	err := SaveThumbnail(context.Background(), client, data, "dest-bucket", "photos/thumbs/test.jpg")
	if err != nil {
		t.Fatalf("SaveThumbnail() error: %v", err)
	}

	if client.putObjectInput == nil {
		t.Fatalf("expected PutObject input to be captured")
	}
	if got := aws.ToString(client.putObjectInput.Bucket); got != "dest-bucket" {
		t.Fatalf("PutObject bucket = %q, want %q", got, "dest-bucket")
	}
	if got := aws.ToString(client.putObjectInput.Key); got != "photos/thumbs/test.jpg" {
		t.Fatalf("PutObject key = %q, want %q", got, "photos/thumbs/test.jpg")
	}
	if got := aws.ToString(client.putObjectInput.ContentType); got != JPEG {
		t.Fatalf("PutObject content type = %q, want %q", got, JPEG)
	}
	if got := aws.ToInt64(client.putObjectInput.ContentLength); got != int64(len(data)) {
		t.Fatalf("PutObject content length = %d, want %d", got, len(data))
	}

	body, err := io.ReadAll(client.putObjectInput.Body)
	if err != nil {
		t.Fatalf("read PutObject body: %v", err)
	}
	if string(body) != string(data) {
		t.Fatalf("PutObject body = %v, want %v", body, data)
	}
}

func TestSaveThumbnailPutObjectError(t *testing.T) {
	t.Parallel()

	client := &fakeS3Client{
		putObjectError: errors.New("put failed"),
	}

	err := SaveThumbnail(context.Background(), client, []byte{1, 2, 3}, "dest-bucket", "photos/thumbs/test.jpg")
	if err == nil {
		t.Fatalf("expected PutObject error")
	}
}

func TestIsSupportedImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ext         string
		contentType string
		want        bool
	}{
		{name: "orf extension", ext: ".orf", want: true},
		{name: "cr3 extension", ext: ".cr3", want: true},
		{name: "heic extension", ext: ".heic", want: true},
		{name: "jpeg content type", contentType: JPEG, want: true},
		{name: "heic content type", contentType: HEIC, want: true},
		{name: "unsupported extension and content type", ext: ".txt", contentType: "text/plain", want: false},
		{name: "empty extension and content type", ext: "", contentType: "", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isSupportedImage(tt.ext, tt.contentType); got != tt.want {
				t.Fatalf("isSupportedImage(%q, %q) = %t, want %t", tt.ext, tt.contentType, got, tt.want)
			}
		})
	}
}
