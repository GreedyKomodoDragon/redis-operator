package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Uploader handles uploading files to AWS S3 using AWS SDK
type S3Uploader struct {
	client   *s3.Client
	uploader *manager.Uploader
	bucket   string
}

// S3Config holds S3 configuration options
type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
}

// NewS3Uploader creates a new S3 uploader using AWS SDK
func NewS3Uploader(ctx context.Context, s3Config S3Config) (*S3Uploader, error) {
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

	// Create uploader with the S3 client
	uploader := manager.NewUploader(s3Client)

	return &S3Uploader{
		client:   s3Client,
		uploader: uploader,
		bucket:   s3Config.Bucket,
	}, nil
}

// UploadFile uploads a file to S3 using AWS SDK
func (u *S3Uploader) UploadFile(ctx context.Context, filePath, keyPrefix string) error {
	// Open the file to upload
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	// Build the S3 key
	fileName := filepath.Base(filePath)
	s3Key := fmt.Sprintf("%s/%s", keyPrefix, fileName)

	// Upload the file
	result, err := u.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(s3Key),
		Body:   file,
		Metadata: map[string]string{
			"uploaded-by": "redis-operator",
			"timestamp":   fmt.Sprintf("%d", time.Now().Unix()),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %v", err)
	}

	fmt.Printf("Successfully uploaded %s to %s\n", fileName, result.Location)
	return nil
}

// uploadToS3Enhanced uploads a file to S3 using AWS SDK with proper error handling
func (s *BackupService) uploadToS3Enhanced(filePath string) error {
	if !s.s3Enabled {
		return nil
	}

	fileName := filepath.Base(filePath)
	fmt.Printf("Uploading %s to S3 bucket %s...\n", fileName, s.s3Bucket)

	// Build S3 configuration from environment variables
	s3Config := S3Config{
		Bucket:          s.s3Bucket,
		Region:          getEnvOrDefault("AWS_REGION", "us-east-1"),
		Endpoint:        os.Getenv("AWS_ENDPOINT_URL"), // Optional custom endpoint
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	// Create uploader with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	uploader, err := NewS3Uploader(ctx, s3Config)
	if err != nil {
		return fmt.Errorf("failed to create S3 uploader: %v", err)
	}

	// Use replication ID as key prefix for organization
	keyPrefix := fmt.Sprintf("redis-backups/%s", s.replID)

	if err := uploader.UploadFile(ctx, filePath, keyPrefix); err != nil {
		return fmt.Errorf("S3 upload failed: %v", err)
	}

	fmt.Printf("Successfully uploaded %s to S3\n", fileName)
	return nil
}
