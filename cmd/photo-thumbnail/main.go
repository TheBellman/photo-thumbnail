package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/adrium/goheif"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/disintegration/imaging"
)

const (
	DefaultSrcPrefix  = "photos/"
	DefaultDestPrefix = "photos/thumbs/"
	DefaultRegion     = "eu-west-2"
	DefaultBucket     = "NOSUCHBUCKET"
	JPEG              = "image/jpeg"
	HEIC              = "image/heic"
	ThumbnailSize     = 200
)

// App holds our dependencies and configuration
type App struct {
	Config     RuntimeConfig
	S3         s3API
	BuildStamp string
}

type RuntimeConfig struct {
	SourcePrefix string
	DestBucket   string
	DestPrefix   string
	Region       string
}

// s3API helps with mocking access to S3
type s3API interface {
	GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// encapsulates the message we get from SNS
type snsMessage struct {
	Records []events.S3EventRecord `json:"Records"`
}

// NewApp initializes the application dependencies including S3 and Wasabi clients.
// It returns an error if the AWS SDK configuration cannot be loaded
func NewApp(ctx context.Context) (*App, error) {
	region := getEnv("AWS_REGION", DefaultRegion)
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	app := &App{
		BuildStamp: os.Getenv("BUILD_STAMP"),
		Config: RuntimeConfig{
			Region:       region,
			SourcePrefix: validatePrefix(os.Getenv("SOURCE_PREFIX"), DefaultSrcPrefix),
			DestBucket:   getEnv("DEST_BUCKET", DefaultBucket),
			DestPrefix:   validatePrefix(os.Getenv("DEST_PREFIX"), DefaultDestPrefix),
		},
		S3: s3.NewFromConfig(cfg),
	}

	return app, nil
}

// getEnv fetches an environmental variable from the lambda environment. If not found
// it falls back on the provided fallback value.
//
// The variable value or the fallback string are returned.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// validatePrefix coerces the environmental variable into a usable prefix, by adding a "/" if necessary or setting it to
// the default prefix. It returns the coerced prefix
func validatePrefix(photoPrefix string, defaultPrefix string) string {
	if !strings.HasSuffix(photoPrefix, "/") {
		if photoPrefix == "" {
			photoPrefix = defaultPrefix
		} else {
			photoPrefix += "/"
		}
	}
	return photoPrefix
}

// getImageReader tries to get an io.Reader exposing the body of an image given the bucket and key. It will fail
// if the provided object is not a supported file type. It returns the reader along with the content type
func getImageReader(ctx context.Context, service s3API, bucket string, key string) (io.Reader, string, error) {
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

// getImage retrieves the byte contents of a specified reader
func getImage(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return data, nil
}

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

// resizeImage attempts to resize the supplied image (assuming the bytes represent a
// jpeg) and hand back a new byte array representing the smaller jpeg
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

// makeThumbKey should replace the old prefix on the key with the new thumbnail prefix
func makeThumbKey(key string, contentType string, sourcePrefix string, destPrefix string) string {
	if contentType == HEIC && strings.EqualFold(path.Ext(key), ".heic") {
		key = strings.TrimSuffix(key, path.Ext(key)) + "_heic.jpg"
	}
	return strings.Replace(key, sourcePrefix, destPrefix, 1)
}

// saveThumbnail tries to save the supplied data to the desired bucket and key.
func saveThumbnail(ctx context.Context, service s3API, data []byte, bucket string, key string) error {
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

// parseMessage tries to forge the JSON message body from SNS
func parseMessage(messageBody string) (*snsMessage, error) {
	var message snsMessage
	err := json.Unmarshal([]byte(messageBody), &message)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the message body: %q = %w", messageBody, err)
	}
	return &message, nil
}

func findRequestId(ctx context.Context) string {
	requestID := ""
	if lc, ok := lambdacontext.FromContext(ctx); ok {
		requestID = lc.AwsRequestID
	}
	return requestID
}

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
