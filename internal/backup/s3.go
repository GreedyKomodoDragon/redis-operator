package backup

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func NewS3Client(ctx context.Context, s3Config S3Config) (*s3.Client, error) {
	var cfg aws.Config
	var err error

	// Build AWS config based on provided credentials and settings
	if s3Config.AccessKeyID != "" && s3Config.SecretAccessKey != "" {
		// Use explicit credentials
		creds := credentials.NewStaticCredentialsProvider(
			s3Config.AccessKeyID,
			s3Config.SecretAccessKey,
			"", // session token
		)

		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithCredentialsProvider(creds),
			config.WithRegion(s3Config.Region),
		)
	} else {
		// Use default AWS credential chain (environment variables, IAM roles, etc.)
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(s3Config.Region),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	// Configure custom endpoint if provided (for S3-compatible services)
	var s3Client *s3.Client
	if s3Config.Endpoint != "" {
		s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s3Config.Endpoint)
			o.UsePathStyle = true // Required for most S3-compatible services
		})
	} else {
		s3Client = s3.NewFromConfig(cfg)
	}

	return s3Client, nil
}
