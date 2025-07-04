package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// S3Uploader handles uploading files to AWS S3 using AWS CLI
type S3Uploader struct {
	bucket string
	region string
}

// NewS3Uploader creates a new S3 uploader using AWS CLI
func NewS3Uploader(bucket, region string) (*S3Uploader, error) {
	// Check if AWS CLI is available
	if err := exec.Command("aws", "--version").Run(); err != nil {
		return nil, fmt.Errorf("AWS CLI not available: %v", err)
	}

	return &S3Uploader{
		bucket: bucket,
		region: region,
	}, nil
}

// UploadFile uploads a file to S3 using AWS CLI
func (u *S3Uploader) UploadFile(ctx context.Context, filePath, keyPrefix string) error {
	fileName := filepath.Base(filePath)
	s3Key := fmt.Sprintf("%s/%s", keyPrefix, fileName)
	s3URI := fmt.Sprintf("s3://%s/%s", u.bucket, s3Key)

	// Create the AWS CLI command
	cmd := exec.CommandContext(ctx, "aws", "s3", "cp", filePath, s3URI, "--region", u.region)

	// Set environment variables for the command
	cmd.Env = os.Environ()

	// Execute the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("AWS CLI upload failed: %v, output: %s", err, string(output))
	}

	return nil
}

// uploadToS3Enhanced uploads a file to S3 using AWS CLI with proper error handling
func (s *BackupService) uploadToS3Enhanced(filePath string) error {
	if !s.s3Enabled {
		return nil
	}

	fileName := filepath.Base(filePath)
	fmt.Printf("Uploading %s to S3 bucket %s...\n", fileName, s.s3Bucket)

	uploader, err := NewS3Uploader(s.s3Bucket, getEnvOrDefault("AWS_REGION", "us-east-1"))
	if err != nil {
		return fmt.Errorf("failed to create S3 uploader: %v", err)
	}

	keyPrefix := fmt.Sprintf("redis-backups/%s", s.replID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := uploader.UploadFile(ctx, filePath, keyPrefix); err != nil {
		return fmt.Errorf("S3 upload failed: %v", err)
	}

	fmt.Printf("Successfully uploaded %s to S3\n", fileName)
	return nil
}
