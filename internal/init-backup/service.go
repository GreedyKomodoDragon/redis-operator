package initbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Service struct {
	s3Client *s3.Client
	bucket   string
	logger   *slog.Logger
}

type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
}

type RDBFile struct {
	Key          string
	LastModified time.Time
	Size         int64
}

func NewService() *Service {
	// Initialize structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	return &Service{
		logger: logger,
	}
}

func (s *Service) Run() error {
	s.logger.Info("Starting init-backup service")

	// Initialize S3 client
	if err := s.initializeS3(); err != nil {
		s.logger.Error("Failed to initialize S3 client", "error", err)
		return err
	}

	// Fetch the latest RDB file
	rdbFile, err := s.fetchLatestRDBFile()
	if err != nil {
		s.logger.Error("Failed to fetch latest RDB file", "error", err)
		return err
	}

	if rdbFile == nil {
		s.logger.Info("No RDB files found in S3 bucket")
		return nil
	}

	s.logger.Info("Latest RDB file found",
		"key", rdbFile.Key,
		"size", rdbFile.Size,
		"last_modified", rdbFile.LastModified,
	)

	// For now, we only discover the latest RDB file
	// Future implementation will download and restore the RDB file
	s.logger.Info("RDB file discovery completed successfully")
	return nil
}

func (s *Service) initializeS3() error {
	// Get S3 configuration from environment variables
	s3Config := S3Config{
		Bucket:          os.Getenv("S3_BUCKET"),
		Region:          getEnvOrDefault("AWS_REGION", "us-east-1"),
		Endpoint:        os.Getenv("AWS_ENDPOINT_URL"),
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	if s3Config.Bucket == "" {
		return fmt.Errorf("S3_BUCKET environment variable is required")
	}

	s.bucket = s3Config.Bucket

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
		return fmt.Errorf("failed to load AWS config: %v", err)
	}

	// Create S3 client
	s3Options := func(o *s3.Options) {
		if s3Config.Endpoint != "" {
			o.BaseEndpoint = aws.String(s3Config.Endpoint)
			o.UsePathStyle = true
		}
	}

	s.s3Client = s3.NewFromConfig(cfg, s3Options)

	s.logger.Info("S3 client initialized",
		"bucket", s.bucket,
		"region", s3Config.Region,
		"custom_endpoint", s3Config.Endpoint != "",
	)

	return nil
}

func (s *Service) fetchLatestRDBFile() (*RDBFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	s.logger.Info("Searching for RDB files in S3 bucket", "bucket", s.bucket)

	// List objects in the bucket with redis-backups prefix
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String("redis-backups/"),
	}

	var rdbFiles []RDBFile

	paginator := s3.NewListObjectsV2Paginator(s.s3Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list S3 objects: %v", err)
		}

		for _, obj := range page.Contents {
			if obj.Key != nil && strings.HasSuffix(*obj.Key, ".rdb") {
				rdbFile := RDBFile{
					Key:          *obj.Key,
					LastModified: *obj.LastModified,
					Size:         *obj.Size,
				}
				rdbFiles = append(rdbFiles, rdbFile)
			}
		}
	}

	if len(rdbFiles) == 0 {
		s.logger.Info("No RDB files found in S3 bucket")
		return nil, nil
	}

	// Sort by last modified time (newest first)
	sort.Slice(rdbFiles, func(i, j int) bool {
		return rdbFiles[i].LastModified.After(rdbFiles[j].LastModified)
	})

	s.logger.Info("Found RDB files",
		"count", len(rdbFiles),
		"latest", rdbFiles[0].Key,
	)

	return &rdbFiles[0], nil
}

// Helper function for environment variables
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
