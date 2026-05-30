package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/TheBellman/photo-thumbnail/internal/processing"
	"github.com/TheBellman/photo-thumbnail/internal/storage"
)

// HandleLambdaEvent takes care of processing the incoming S3 event. Only "ObjectCreated:*" events are processed, and only
// where the object key starts with the nominated prefix. The count of processed objects is returned.
func (a *App) HandleLambdaEvent(ctx context.Context, snsEvent events.SNSEvent) (int, error) {
	cnt := 0
	logger := slog.With("build_stamp", a.BuildStamp, "request_id", findRequestID(ctx))

	for _, record := range snsEvent.Records {
		message, err := parseMessage(record.SNS.Message)
		if err != nil {
			logger.Error("failed to parse SNS message", "error", err)
			continue
		}

		for _, event := range message.Records {
			logger.Info("received request", "bucket", event.S3.Bucket.Name, "key", event.S3.Object.Key)

			if !a.shouldProcessEvent(event) {
				continue
			}

			decodedKey, ok := a.validateEvent(event, logger)
			if !ok {
				continue
			}

			if err := a.processImage(ctx, event.S3.Bucket.Name, decodedKey); err != nil {
				logger.Error("failed to process image", "bucket", event.S3.Bucket.Name, "key", decodedKey, "error", err)
				continue
			}

			logger.Info("processed request", "bucket", event.S3.Bucket.Name, "key", decodedKey)
			cnt++
		}
	}

	return cnt, nil
}

// shouldProcessEvent determines if an S3 event should be processed based on the object's key prefix and event name.
func (a *App) shouldProcessEvent(event events.S3EventRecord) bool {
	return strings.HasPrefix(event.S3.Object.Key, a.Config.SourcePrefix) &&
		strings.HasPrefix(event.EventName, "ObjectCreated:")
}

// validateEvent validates an S3 event by decoding its object key and verifying the AWS region matches the configuration.
func (a *App) validateEvent(event events.S3EventRecord, logger *slog.Logger) (string, bool) {
	decodedKey, err := url.QueryUnescape(event.S3.Object.Key)
	if err != nil {
		logger.Error("failed to decode key", "key", event.S3.Object.Key, "error", err)
		return "", false
	}

	if event.AWSRegion != a.Config.Region {
		logger.Warn("event from unexpected region", "event_region", event.AWSRegion, "lambda_region", a.Config.Region)
		return "", false
	}

	return decodedKey, true
}

// processImage processes an image from the specified S3 bucket and key, generates a thumbnail, and saves it to the destination.
func (a *App) processImage(ctx context.Context, bucket string, key string) error {
	imgReader, contentType, err := storage.GetImageReader(ctx, a.S3, bucket, key)
	if err != nil {
		return fmt.Errorf("get image reader: %w", err)
	}

	thumbBytes, err := processing.CreateThumbnail(key, imgReader, contentType, ThumbnailSize)
	if err != nil {
		return err
	}

	thumbKey := makeThumbKey(key, contentType, a.Config.SourcePrefix, a.Config.DestPrefix)
	if err = storage.SaveThumbnail(ctx, a.S3, thumbBytes, a.Config.DestBucket, thumbKey); err != nil {
		return fmt.Errorf("save thumbnail: %w", err)
	}

	return nil
}
