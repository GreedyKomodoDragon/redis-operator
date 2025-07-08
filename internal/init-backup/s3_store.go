package initbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Store implements ObjectStore for AWS S3 or S3-compatible storage
type S3Store struct {
	client *s3.Client
	bucket string
	logger *slog.Logger
}

// NewS3Store creates a new S3Store instance
func NewS3Store(logger *slog.Logger) (*S3Store, error) {
	// Get S3 configuration from environment variables
	s3Config := S3Config{
		Bucket:          os.Getenv("S3_BUCKET"),
		Region:          getEnvOrDefault("AWS_REGION", "us-east-1"),
		Endpoint:        os.Getenv("AWS_ENDPOINT_URL"),
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	if s3Config.Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET environment variable is required")
	}

	// Create AWS config
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var cfg aws.Config
	var err error

	if s3Config.Endpoint != "" {
		// Custom endpoint (e.g., MinIO)
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(s3Config.Region),
			config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     s3Config.AccessKeyID,
					SecretAccessKey: s3Config.SecretAccessKey,
				}, nil
			})),
		)
	} else {
		// Standard AWS
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(s3Config.Region),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	// Create S3 client
	s3Options := func(o *s3.Options) {
		if s3Config.Endpoint != "" {
			o.BaseEndpoint = aws.String(s3Config.Endpoint)
			o.UsePathStyle = true
		}
	}

	client := s3.NewFromConfig(cfg, s3Options)

	logger.Info("S3 client initialized",
		"bucket", s3Config.Bucket,
		"region", s3Config.Region,
		"custom_endpoint", s3Config.Endpoint != "",
	)

	return &S3Store{
		client: client,
		bucket: s3Config.Bucket,
		logger: logger,
	}, nil
}

// ListFiles implements ObjectStore.ListFiles
func (s *S3Store) ListFiles(ctx context.Context, prefix string) ([]ObjectStoreFile, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	var files []ObjectStoreFile

	paginator := s3.NewListObjectsV2Paginator(s.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list S3 objects: %v", err)
		}

		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}

			files = append(files, ObjectStoreFile{
				Key:          *obj.Key,
				LastModified: *obj.LastModified,
				Size:         *obj.Size,
			})
		}
	}

	return files, nil
}

// DownloadFile implements ObjectStore.DownloadFile
func (s *S3Store) DownloadFile(ctx context.Context, key string) ([]byte, error) {
	// TODO: Implement download functionality for future restore feature
	return nil, fmt.Errorf("download functionality not implemented yet")
}

// GetBucketName implements ObjectStore.GetBucketName
func (s *S3Store) GetBucketName() string {
	return s.bucket
}

// Close implements ObjectStore.Close
func (s *S3Store) Close() error {
	// S3 client doesn't require explicit cleanup
	return nil
}
