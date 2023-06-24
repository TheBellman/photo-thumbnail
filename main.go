package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/disintegration/imaging"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
)

// s3Service helps with mocking access to S3
type s3Service interface {
	GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error)
	PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error)
}

// encapsulates the message we get from SNS
type snsMessage struct {
	Records []events.S3EventRecord `json:"Records"`
}

var params *runtimeParameters
var buildStamp string

const (
	DefaultSrcPrefix  = "photos/"
	DefaultDestPrefix = "photos/thumbs/"
	DefaultRegion     = "eu-west-2"
	DefaultBucket     = "NOSUCHBUCKET"
	JPEG              = "image/jpeg"
	ThumbnailSize     = 200
)

// runtimeParameters contains various bits needed during execution
type runtimeParameters struct {
	Region       string
	SourceBucket string
	SourcePrefix string
	DestBucket   string
	DestPrefix   string
	S3service    *s3.S3
}

func init() {
	buildStamp = os.Getenv("BUILD_STAMP")
	params = &runtimeParameters{
		SourcePrefix: validatePrefix(os.Getenv("SOURCE_PREFIX"), DefaultSrcPrefix),
		DestPrefix:   validatePrefix(os.Getenv("DEST_PREFIX"), DefaultDestPrefix),
		DestBucket:   validateDestination(os.Getenv("DESTINATION_BUCKET"), DefaultBucket),
		Region:       validateRegion(os.Getenv("AWS_REGION"), DefaultRegion),
	}
}

// validateDestination will ensure a non-blank destination bucket
func validateDestination(bucket string, defaultBucket string) string {
	if bucket == "" {
		return defaultBucket
	} else {
		return bucket
	}
}

// validateRegion will provide the default region if no region is set
func validateRegion(region string, defaultRegion string) string {
	if region == "" {
		return defaultRegion
	} else {
		return region
	}
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

// makeAWSSession sets up an AWS session that can be used to connect to S3.
func makeAWSSession(region string) (*session.Session, error) {
	return session.NewSession(
		&aws.Config{
			Region: aws.String(region),
		})
}

// getImageReader tries to get an io.Reader exposing the body of an image given the bucket and key. It will fail
// if the provided object is not a supported file type
func getImageReader(service s3Service, bucket string, key string) (io.Reader, error) {
	result, err := service.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("error fetching from s3: %v", err)
	}

	if strings.HasSuffix(strings.ToLower(key), ".cr3") || *result.ContentType == JPEG {
		return result.Body, nil
	}
	return nil, fmt.Errorf("only JPEG and CR3 supported, fetched file %s was reported as %s",
		key,
		*result.ContentType)
}

// getImage retrieves the byte contents of a specified reader
func getImage(r io.Reader) (*[]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return &[]byte{}, err
	}
	return &data, nil
}

// resizeImage attempts to resize the supplied image (assuming the bytes represent a
// jpeg) and hand back a new byte array representing the smaller jpeg
func resizeImage(origImg *[]byte) (*[]byte, error) {
	imgConf, _, err := image.DecodeConfig(bytes.NewReader(*origImg))
	if err != nil {
		return &[]byte{}, fmt.Errorf("failed to decode byte stream as a jpeg: %v", err)
	}

	original, _, err := image.Decode(bytes.NewReader(*origImg))
	if err != nil {
		return &[]byte{}, fmt.Errorf("failed to decode byte stream as a jpeg: %v", err)
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
		return &[]byte{}, fmt.Errorf("failed to encode resized image as jpeg: %v", err)
	}

	result := buf.Bytes()
	return &result, nil
}

// makeThumbKey should replace the old prefix on the key with the new thumbnail prefix
func makeThumbKey(key string) string {
	return strings.Replace(key, params.SourcePrefix, DefaultDestPrefix, 1)
}

// saveThumbnail tries to save the supplied data to the desired bucket and key.
func saveThumbnail(service s3Service, data *[]byte, bucket string, key string) error {
	reader := bytes.NewReader(*data)
	_, err := service.PutObject(&s3.PutObjectInput{
		Body:          reader,
		Bucket:        aws.String(bucket),
		ContentLength: aws.Int64(int64(len(*data))),
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
		return nil, fmt.Errorf("failed to parse the message body: %q = %v", messageBody, err)
	}
	return &message, nil
}

// HandleLambdaEvent takes care of processing the incoming S3 event. Only "ObjectCreated:*" events are processed, and only
// for where the object key starts with the nominated prefix. The count of processed objects is returned
func HandleLambdaEvent(snsEvent events.SNSEvent) (int, error) {
	cnt := 0
	// each SNS event probably only has a single record in it, but you never know
	for _, record := range snsEvent.Records {
		message, err := parseMessage(record.SNS.Message)
		if err != nil {
			log.Printf("[%s] failed to parse the SNS message at all: %v", buildStamp, err)
			continue
		}

		// each SNS event record is an S3EventRecord
		for _, event := range message.Records {
			log.Printf("[%s] Received request for : object %s/%s", buildStamp, event.S3.Bucket.Name, event.S3.Object.Key)
			// only process events where the object key as the expected prefix and the event is an object creation
			if strings.HasPrefix(event.S3.Object.Key, params.SourcePrefix) && strings.HasPrefix(event.EventName, "ObjectCreated:") {
				decodedKey, err := url.QueryUnescape(event.S3.Object.Key)
				if err != nil {
					log.Printf("[%s] Failed to decode the key: '%s'", buildStamp, event.S3.Object.Key)
					continue
				}

				// this should be a cannot-happen case
				if event.AWSRegion != params.Region {
					log.Printf("[%s] Event is not from the same region as the lambda: got %q, wanted %q", buildStamp, event.AWSRegion, params.Region)
					continue
				}

				if strings.HasSuffix(strings.ToLower(decodedKey), ".cr3") {
					log.Printf("[%s] skipping %s until we can figure out how to handle RAW", buildStamp, decodedKey)
					continue
				}

				// fetch the object and hand back an io.reader
				imgReader, err := getImageReader(params.S3service, event.S3.Bucket.Name, decodedKey)
				if err != nil {
					log.Printf("[%s] Failed to get a reader to read from %s/%s: %v", buildStamp, event.S3.Bucket.Name, decodedKey, err)
					continue
				}

				// extract the image data
				imageBytes, err := getImage(imgReader)
				if err != nil {
					log.Printf("[%s] Failed to read image bytes: %v", buildStamp, err)
					continue
				}

				// create a thumbnail from our image bytes, getting back a *byte[]
				thumbBytes, err := resizeImage(imageBytes)
				if err != nil {
					log.Printf("[%s] failed to create a thumbnail image: %v", buildStamp, err)
					continue
				}

				if err = saveThumbnail(params.S3service, thumbBytes, params.DestBucket, makeThumbKey(decodedKey)); err != nil {
					log.Printf("[%s] failed to save the thumbnail: %v", buildStamp, err)
					continue
				}

				log.Printf("[%s] Processed request for : object %s/%s", buildStamp, event.S3.Bucket.Name, decodedKey)
				cnt++
			}
		}
	}

	return cnt, nil
}

// main function invoked when the lambda is launched
func main() {
	// create a service to read from S3
	sess, err := makeAWSSession(params.Region)
	if err != nil {
		log.Fatal("Error starting AWS session", err)
	}
	params.S3service = s3.New(sess)

	log.Printf("[%s] Registering handler for photo-thumbnail...", buildStamp)
	lambda.Start(HandleLambdaEvent)
}
