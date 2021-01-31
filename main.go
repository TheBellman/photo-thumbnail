package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"log"
	"os"
	"strings"
)

// s3Service helps with mocking access to S3
type s3Service interface {
	GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

var params *runtimeParameters


const (
	DefaultSrcPrefix = "import/"
	DefaultDestPrefix = "thumbs/"
	DefaultRegion    = "eu-west-2"
	//JPEG             = "image/jpeg"
)

func init() {
	params = &runtimeParameters{
		SourcePrefix: validatePrefix(os.Getenv("SOURCE_PREFIX"), DefaultSrcPrefix),
		DestPrefix: validatePrefix(os.Getenv("DEST_PREFIX"), DefaultDestPrefix),
		Region:       validateRegion(os.Getenv("AWS_REGION")),
	}
}


// validateRegion will provide the default region if no region is set
func validateRegion(region string) string {
	if region == "" {
		return DefaultRegion
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

// runtimeParameters contains various bits needed during execution
type runtimeParameters struct {
	Region        string
	SourceBucket  string
	SourcePrefix  string
	DestPrefix    string
	S3service     *s3.S3
}

// makeAWSSession sets up an AWS session that can be used to connect to S3.
func makeAWSSession(region string) (*session.Session, error) {
	return session.NewSession(
		&aws.Config{
			Region: aws.String(region),
		})
}

// main function invoked when the lambda is launched
func main() {
	// create a service to read from S3
	sess, err := makeAWSSession(params.Region)
	if err != nil {
		log.Fatal("Error starting AWS session", err)
	}
	params.S3service = s3.New(sess)
}
