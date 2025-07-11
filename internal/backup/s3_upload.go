package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Uploader handles uploading files to AWS S3 using AWS SDK and implements the Uploader interface
type S3Uploader struct {
	Client   *s3.Client
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
func NewS3Uploader(ctx context.Context, s3Client *s3.Client, s3Bucket string) *S3Uploader {
	return &S3Uploader{
		Client:   s3Client,
		uploader: manager.NewUploader(s3Client),
		bucket:   s3Bucket,
	}
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

	slog.Info("Successfully uploaded file", "fileName", fileName, "location", result.Location)
	return nil
}

// UploadStream uploads data from an io.Reader directly to S3
func (u *S3Uploader) UploadStream(ctx context.Context, reader io.Reader, s3Key string, metadata map[string]string) (string, error) {
	// Upload the stream
	result, err := u.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:   aws.String(u.bucket),
		Key:      aws.String(s3Key),
		Body:     reader,
		Metadata: metadata,
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload stream to S3: %v", err)
	}

	return result.Location, nil
}

// UploadStreamWithProgress uploads data from an io.Reader to S3 with progress tracking
func (u *S3Uploader) UploadStreamWithProgress(ctx context.Context, reader io.Reader, s3Key string, metadata map[string]string, progressCallback func(bytes int64)) (string, error) {
	// Wrap the reader with progress tracking
	progressReader := &ProgressReader{
		Reader:   reader,
		Callback: progressCallback,
	}

	return u.UploadStream(ctx, progressReader, s3Key, metadata)
}

// ProgressReader wraps an io.Reader to track progress
type ProgressReader struct {
	Reader   io.Reader
	Callback func(bytes int64)
	total    int64
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	if n > 0 {
		pr.total += int64(n)
		if pr.Callback != nil {
			pr.Callback(pr.total)
		}
	}
	return n, err
}
