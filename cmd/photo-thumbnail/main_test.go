package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3 struct {
	getCalled bool
	putCalled bool
}

func (f *fakeS3) GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.getCalled = true
	return nil, errors.New("unexpected GetObject call")
}

func (f *fakeS3) PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putCalled = true
	return nil, errors.New("unexpected PutObject call")
}

func TestValidatePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{name: "empty uses fallback", input: "", fallback: "photos/", want: "photos/"},
		{name: "adds slash", input: "photos", fallback: "photos/", want: "photos/"},
		{name: "keeps slash", input: "photos/", fallback: "photos/", want: "photos/"},
		{name: "nested adds slash", input: "x/y", fallback: "photos/", want: "x/y/"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := validatePrefix(tt.input, tt.fallback); got != tt.want {
				t.Fatalf("validatePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMakeThumbKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		key          string
		contentType  string
		sourcePrefix string
		destPrefix   string
		want         string
	}{
		{
			name:         "heic converts to jpeg",
			key:          "photos/IMG_0001.HEIC",
			contentType:  HEIC,
			sourcePrefix: "photos/",
			destPrefix:   "photos/thumbs/",
			want:         "photos/thumbs/IMG_0001_heic.jpg",
		},
		{
			name:         "jpeg keeps name",
			key:          "photos/IMG_0002.jpg",
			contentType:  JPEG,
			sourcePrefix: "photos/",
			destPrefix:   "photos/thumbs/",
			want:         "photos/thumbs/IMG_0002.jpg",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := makeThumbKey(tt.key, tt.contentType, tt.sourcePrefix, tt.destPrefix); got != tt.want {
				t.Fatalf("makeThumbKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestHandleLambdaEventFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record events.S3EventRecord
	}{
		{
			name: "event name mismatch",
			record: events.S3EventRecord{
				EventName: "ObjectRemoved:Delete",
				AWSRegion: "us-east-1",
				S3: events.S3Entity{
					Bucket: events.S3Bucket{Name: "bucket"},
					Object: events.S3Object{Key: "photos/test.jpg"},
				},
			},
		},
		{
			name: "prefix mismatch",
			record: events.S3EventRecord{
				EventName: "ObjectCreated:Put",
				AWSRegion: "us-east-1",
				S3: events.S3Entity{
					Bucket: events.S3Bucket{Name: "bucket"},
					Object: events.S3Object{Key: "other/test.jpg"},
				},
			},
		},
		{
			name: "region mismatch",
			record: events.S3EventRecord{
				EventName: "ObjectCreated:Put",
				AWSRegion: "us-west-2",
				S3: events.S3Entity{
					Bucket: events.S3Bucket{Name: "bucket"},
					Object: events.S3Object{Key: "photos/test.jpg"},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			msg := snsMessage{Records: []events.S3EventRecord{tt.record}}
			body, err := json.Marshal(msg)
			if err != nil {
				t.Fatalf("marshal message: %v", err)
			}

			snsEvent := events.SNSEvent{
				Records: []events.SNSEventRecord{
					{
						SNS: events.SNSEntity{Message: string(body)},
					},
				},
			}

			fake := &fakeS3{}
			app := &App{
				Config: RuntimeConfig{
					Region:       "us-east-1",
					SourcePrefix: "photos/",
					DestBucket:   "dest-bucket",
					DestPrefix:   "photos/thumbs/",
				},
				S3: fake,
			}

			count, err := app.HandleLambdaEvent(context.Background(), snsEvent)
			if err != nil {
				t.Fatalf("HandleLambdaEvent returned error: %v", err)
			}
			if count != 0 {
				t.Fatalf("HandleLambdaEvent count = %d, want 0", count)
			}
			if fake.getCalled || fake.putCalled {
				t.Fatalf("unexpected S3 calls: get=%t put=%t", fake.getCalled, fake.putCalled)
			}
		})
	}
}
