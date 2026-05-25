package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/TheBellman/photo-thumbnail/internal/storage"
)

const (
	DefaultSrcPrefix  = "photos/"
	DefaultDestPrefix = "photos/thumbs/"
	DefaultRegion     = "eu-west-2"
	DefaultBucket     = "NOSUCHBUCKET"
	ThumbnailSize     = 200
)

// App holds our dependencies and configuration.
type App struct {
	Config     RuntimeConfig
	S3         storage.Client
	BuildStamp string
}

// RuntimeConfig is a handy container for all the runtime parameters injected from the lambda environment.
type RuntimeConfig struct {
	SourcePrefix string
	DestBucket   string
	DestPrefix   string
	Region       string
}

// New initializes the application dependencies including the S3 client.
//
// It returns an error if the AWS SDK configuration cannot be loaded.
func New(ctx context.Context) (*App, error) {
	region := getEnv("AWS_REGION", DefaultRegion)
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return &App{
		BuildStamp: os.Getenv("BUILD_STAMP"),
		Config: RuntimeConfig{
			Region:       region,
			SourcePrefix: validatePrefix(os.Getenv("SOURCE_PREFIX"), DefaultSrcPrefix),
			DestBucket:   getEnv("DEST_BUCKET", DefaultBucket),
			DestPrefix:   validatePrefix(os.Getenv("DEST_PREFIX"), DefaultDestPrefix),
		},
		S3: s3.NewFromConfig(cfg),
	}, nil
}

// getEnv fetches an environmental variable from the lambda environment. If not found
// it falls back on the provided fallback value.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// validatePrefix coerces the environmental variable into a usable prefix, by adding a "/" if necessary or setting it to
// the default prefix.
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
