package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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

func TestShouldProcessEvent(t *testing.T) {
	t.Parallel()

	application := &App{
		Config: RuntimeConfig{
			SourcePrefix: "photos/",
		},
	}

	tests := []struct {
		name  string
		event events.S3EventRecord
		want  bool
	}{
		{
			name: "object created with matching prefix",
			event: events.S3EventRecord{
				EventName: "ObjectCreated:Put",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "photos/test.jpg"},
				},
			},
			want: true,
		},
		{
			name: "event name mismatch",
			event: events.S3EventRecord{
				EventName: "ObjectRemoved:Delete",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "photos/test.jpg"},
				},
			},
			want: false,
		},
		{
			name: "prefix mismatch",
			event: events.S3EventRecord{
				EventName: "ObjectCreated:Put",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "other/test.jpg"},
				},
			},
			want: false,
		},
		{
			name: "case sensitive prefix mismatch",
			event: events.S3EventRecord{
				EventName: "ObjectCreated:Put",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "Photos/test.jpg"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := application.shouldProcessEvent(tt.event); got != tt.want {
				t.Fatalf("shouldProcessEvent() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestValidateEvent(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	application := &App{
		Config: RuntimeConfig{
			Region: "eu-west-2",
		},
	}

	tests := []struct {
		name    string
		event   events.S3EventRecord
		wantKey string
		wantOK  bool
	}{
		{
			name: "valid event decodes key",
			event: events.S3EventRecord{
				AWSRegion: "eu-west-2",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "photos/My+Photo.jpg"},
				},
			},
			wantKey: "photos/My Photo.jpg",
			wantOK:  true,
		},
		{
			name: "region mismatch",
			event: events.S3EventRecord{
				AWSRegion: "us-east-1",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "photos/test.jpg"},
				},
			},
			wantKey: "",
			wantOK:  false,
		},
		{
			name: "invalid escaped key",
			event: events.S3EventRecord{
				AWSRegion: "eu-west-2",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "photos/%zz.jpg"},
				},
			},
			wantKey: "",
			wantOK:  false,
		},
		{
			name: "cr3 is valid",
			event: events.S3EventRecord{
				AWSRegion: "eu-west-2",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "photos/test.CR3"},
				},
			},
			wantKey: "photos/test.CR3",
			wantOK:  true,
		},
		{
			name: "orf is valid",
			event: events.S3EventRecord{
				AWSRegion: "eu-west-2",
				S3: events.S3Entity{
					Object: events.S3Object{Key: "photos/test.ORF"},
				},
			},
			wantKey: "photos/test.ORF",
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotKey, gotOK := application.validateEvent(tt.event, logger)
			if gotOK != tt.wantOK {
				t.Fatalf("validateEvent() ok = %t, want %t", gotOK, tt.wantOK)
			}
			if gotKey != tt.wantKey {
				t.Fatalf("validateEvent() key = %q, want %q", gotKey, tt.wantKey)
			}
		})
	}
}
