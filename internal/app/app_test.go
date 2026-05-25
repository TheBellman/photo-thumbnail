package app

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
			application := &App{
				Config: RuntimeConfig{
					Region:       "us-east-1",
					SourcePrefix: "photos/",
					DestBucket:   "dest-bucket",
					DestPrefix:   "photos/thumbs/",
				},
				S3: fake,
			}

			count, err := application.HandleLambdaEvent(context.Background(), snsEvent)
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
