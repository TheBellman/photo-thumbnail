package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// HandleLambdaEvent takes care of processing the incoming S3 event. Only "ObjectCreated:*" events are processed, and only
// for where the object key starts with the nominated prefix. The count of processed objects is returned
func (a *App) HandleLambdaEvent(ctx context.Context, snsEvent events.SNSEvent) (int, error) {
	cnt := 0
	logger := slog.With("build_stamp", a.BuildStamp, "request_id", findRequestId(ctx))
	// each SNS event probably only has a single record in it, but you never know
	for _, record := range snsEvent.Records {
		message, err := parseMessage(record.SNS.Message)
		if err != nil {
			logger.Error("failed to parse SNS message", "error", err)
			continue
		}

		// each SNS event record is an S3EventRecord
		for _, event := range message.Records {
			logger.Info("received request", "bucket", event.S3.Bucket.Name, "key", event.S3.Object.Key)
			// only process events where the object key as the expected prefix and the event is an object creation
			if strings.HasPrefix(event.S3.Object.Key, a.Config.SourcePrefix) && strings.HasPrefix(event.EventName, "ObjectCreated:") {
				decodedKey, err := url.QueryUnescape(event.S3.Object.Key)
				if err != nil {
					logger.Error("failed to decode key", "key", event.S3.Object.Key, "error", err)
					continue
				}

				// this should be a cannot-happen case
				if event.AWSRegion != a.Config.Region {
					logger.Warn("event from unexpected region", "event_region", event.AWSRegion, "lambda_region", a.Config.Region)
					continue
				}

				if strings.EqualFold(path.Ext(decodedKey), ".cr3") {
					logger.Info("skipping RAW file", "key", decodedKey)
					continue
				}

				// fetch the object and hand back an io.reader and the content type
				imgReader, contentType, err := getImageReader(ctx, a.S3, event.S3.Bucket.Name, decodedKey)
				if err != nil {
					logger.Error("failed to get image reader", "bucket", event.S3.Bucket.Name, "key", decodedKey, "error", err)
					continue
				}

				var imageBytes []byte
				if contentType == HEIC {
					imageBytes, err = convertHeicToJpeg(imgReader)
					if err != nil {
						logger.Error("failed to convert HEIC to JPEG", "key", decodedKey, "error", err)
						continue
					}
				} else {
					// extract the image data
					imageBytes, err = getImage(imgReader)
					if err != nil {
						logger.Error("failed to read image bytes", "key", decodedKey, "error", err)
						continue
					}
				}

				// create a thumbnail from our image bytes, getting back a []byte
				thumbBytes, err := resizeImage(imageBytes)
				if err != nil {
					logger.Error("failed to create thumbnail image", "key", decodedKey, "error", err)
					continue
				}

				if err = saveThumbnail(ctx, a.S3, thumbBytes, a.Config.DestBucket, makeThumbKey(decodedKey, contentType, a.Config.SourcePrefix, a.Config.DestPrefix)); err != nil {
					logger.Error("failed to save thumbnail", "bucket", a.Config.DestBucket, "key", decodedKey, "error", err)
					continue
				}

				logger.Info("processed request", "bucket", event.S3.Bucket.Name, "key", decodedKey)
				cnt++
			}
		}
	}

	return cnt, nil
}

// main function invoked when the lambda is launched
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()
	app, err := NewApp(ctx)
	if err != nil {
		slog.Error("Initialization failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Starting photo-thumbnail handler", "build_stamp", app.BuildStamp)
	lambda.Start(app.HandleLambdaEvent)
}
