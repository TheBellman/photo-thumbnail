package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/TheBellman/photo-thumbnail/internal/app"
)

// main function invoked when the lambda is launched
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()
	application, err := app.New(ctx)
	if err != nil {
		slog.Error("initialization failed", "error", err)
		os.Exit(1)
	}

	slog.Info("starting photo-thumbnail handler", "build_stamp", application.BuildStamp)
	lambda.Start(application.HandleLambdaEvent)
}
