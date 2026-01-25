package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

// snsMessage encapsulates the message we get from SNS
type snsMessage struct {
	Records []events.S3EventRecord `json:"Records"`
}

// parseMessage tries to forge the JSON message body from SNS.
//
// It returns the actual message body, or an error if it fails to parse it.
func parseMessage(messageBody string) (*snsMessage, error) {
	var message snsMessage
	err := json.Unmarshal([]byte(messageBody), &message)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the message body: %q = %w", messageBody, err)
	}
	return &message, nil
}

// findRequestId is used to find the runtime request ID from the lambda context for the current run.
//
// This returns the discovered request ID or an empty string
func findRequestId(ctx context.Context) string {
	if lc, ok := lambdacontext.FromContext(ctx); ok {
		return lc.AwsRequestID
	}
	return ""
}

// makeThumbKey should replace the old prefix on the key with the new thumbnail prefix.
//
// The updated object key is returned.
func makeThumbKey(key string, contentType string, sourcePrefix string, destPrefix string) string {
	if contentType == HEIC && strings.EqualFold(path.Ext(key), ".heic") {
		key = strings.TrimSuffix(key, path.Ext(key)) + "_heic.jpg"
	}
	return strings.Replace(key, sourcePrefix, destPrefix, 1)
}
