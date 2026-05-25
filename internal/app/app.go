package app

import (
	"context"
	"log/slog"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	thumbnailimage "github.com/TheBellman/photo-thumbnail/internal/image"
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

			if !strings.HasPrefix(event.S3.Object.Key, a.Config.SourcePrefix) ||
				!strings.HasPrefix(event.EventName, "ObjectCreated:") {
				continue
			}

			decodedKey, err := url.QueryUnescape(event.S3.Object.Key)
			if err != nil {
				logger.Error("failed to decode key", "key", event.S3.Object.Key, "error", err)
				continue
			}

			if event.AWSRegion != a.Config.Region {
				logger.Warn("event from unexpected region", "event_region", event.AWSRegion, "lambda_region", a.Config.Region)
				continue
			}

			if strings.EqualFold(path.Ext(decodedKey), ".cr3") {
				logger.Info("skipping RAW file", "key", decodedKey)
				continue
			}

			imgReader, contentType, err := storage.GetImageReader(ctx, a.S3, event.S3.Bucket.Name, decodedKey)
			if err != nil {
				logger.Error("failed to get image reader", "bucket", event.S3.Bucket.Name, "key", decodedKey, "error", err)
				continue
			}

			var imageBytes []byte
			if contentType == storage.HEIC {
				imageBytes, err = thumbnailimage.ConvertHEICToJPEG(imgReader)
				if err != nil {
					logger.Error("failed to convert HEIC to JPEG", "key", decodedKey, "error", err)
					continue
				}
			} else {
				imageBytes, err = thumbnailimage.Read(imgReader)
				if err != nil {
					logger.Error("failed to read image bytes", "key", decodedKey, "error", err)
					continue
				}
			}

			thumbBytes, err := thumbnailimage.Resize(imageBytes, ThumbnailSize)
			if err != nil {
				logger.Error("failed to create thumbnail image", "key", decodedKey, "error", err)
				continue
			}

			thumbKey := makeThumbKey(decodedKey, contentType, a.Config.SourcePrefix, a.Config.DestPrefix)
			if err = storage.SaveThumbnail(ctx, a.S3, thumbBytes, a.Config.DestBucket, thumbKey); err != nil {
				logger.Error("failed to save thumbnail", "bucket", a.Config.DestBucket, "key", decodedKey, "error", err)
				continue
			}

			logger.Info("processed request", "bucket", event.S3.Bucket.Name, "key", decodedKey)
			cnt++
		}
	}

	return cnt, nil
}
