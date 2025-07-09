package backup_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/GreedyKomodoDragon/redis-operator/internal/backup"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

const (
	objectManagerTestBucket = "object-manager-test-bucket"
	objectManagerMinioImage = "minio/minio:RELEASE.2024-01-16T16-07-38Z"
)

// setupMinIOForObjectManager sets up MinIO container for object manager tests
func setupMinIOForObjectManager(t *testing.T) (context.Context, *s3.Client, func()) {
	ctx := context.Background()

	// Start MinIO container
	minioContainer, err := minio.Run(ctx, objectManagerMinioImage)
	require.NoError(t, err)

	// Get MinIO connection info
	endpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Format endpoint with http:// protocol
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	// Create S3 client
	s3Client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "us-east-1",
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "minioadmin",
				SecretAccessKey: "minioadmin",
			}, nil
		}),
		UsePathStyle: true,
	})

	// Create bucket
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(objectManagerTestBucket),
	})
	require.NoError(t, err)

	cleanup := func() {
		if err := testcontainers.TerminateContainer(minioContainer); err != nil {
			t.Logf("failed to terminate MinIO container: %s", err)
		}
	}

	return ctx, s3Client, cleanup
}

// TestBackupObjectManagerIntegrationWithMinIO tests the complete integration with MinIO
func TestBackupObjectManagerIntegrationWithMinIO(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForObjectManager(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create real uploader and retention manager
	uploader := backup.NewS3Uploader(ctx, s3Client, objectManagerTestBucket)
	retentionManager := backup.NewS3RetentionManager(s3Client, objectManagerTestBucket, logger)

	objectManager := backup.NewBackupObjectManager(uploader, retentionManager, logger)

	// Test data
	reader := strings.NewReader("test backup content")
	key := "test-backup-key"
	metadata := map[string]string{"type": "rdb"}
	replicationID := "test-replication-id"
	retentionCount := 3

	// Track progress calls
	var progressCalls []int64
	progressCallback := func(bytes int64) {
		progressCalls = append(progressCalls, bytes)
	}

	// Execute the test
	err := objectManager.UploadWithProgressAndRetention(ctx, reader, key, metadata, replicationID, retentionCount, progressCallback)

	// Verify results
	require.NoError(t, err)
	assert.NotEmpty(t, progressCalls, "Progress callback should be called")

	// Verify file was uploaded
	resp, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(objectManagerTestBucket),
		Key:    aws.String(key),
	})
	require.NoError(t, err)

	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "test backup content", string(content))

	// Verify metadata
	assert.Equal(t, "rdb", resp.Metadata["type"])
}

// TestBackupObjectManagerMultipleUploadsWithRetention tests multiple uploads with retention
func TestBackupObjectManagerMultipleUploadsWithRetention(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForObjectManager(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create real uploader and retention manager
	uploader := backup.NewS3Uploader(ctx, s3Client, objectManagerTestBucket)
	retentionManager := backup.NewS3RetentionManager(s3Client, objectManagerTestBucket, logger)

	objectManager := backup.NewBackupObjectManager(uploader, retentionManager, logger)

	replicationID := "test-replication-id"
	retentionCount := 2

	// Upload multiple files with different timestamps
	timestamps := []string{
		"20240101-100000",
		"20240101-110000",
		"20240101-120000",
		"20240101-130000", // This should trigger retention
	}

	for i, timestamp := range timestamps {
		key := fmt.Sprintf("redis-backups/%s/dump-%s.rdb", replicationID, timestamp)
		content := fmt.Sprintf("backup content %d", i)
		reader := strings.NewReader(content)
		metadata := map[string]string{"type": "rdb"}

		err := objectManager.UploadWithProgressAndRetention(ctx, reader, key, metadata, replicationID, retentionCount, func(int64) {
			// Progress callback - no action needed for test
		})
		require.NoError(t, err)
	}

	// Verify retention was applied - only 2 files should remain
	listResp, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(objectManagerTestBucket),
		Prefix: aws.String(fmt.Sprintf("redis-backups/%s/", replicationID)),
	})
	require.NoError(t, err)

	assert.Equal(t, 2, len(listResp.Contents), "Should have exactly 2 files after retention")

	// Verify the correct files remain (newest ones)
	var remainingFiles []string
	for _, obj := range listResp.Contents {
		remainingFiles = append(remainingFiles, *obj.Key)
	}

	expectedFiles := []string{
		fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replicationID),
		fmt.Sprintf("redis-backups/%s/dump-20240101-130000.rdb", replicationID),
	}

	for _, expectedFile := range expectedFiles {
		assert.Contains(t, remainingFiles, expectedFile)
	}
}

// TestBackupObjectManagerConcurrentUploads tests concurrent uploads
func TestBackupObjectManagerConcurrentUploads(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForObjectManager(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create real uploader and retention manager
	uploader := backup.NewS3Uploader(ctx, s3Client, objectManagerTestBucket)
	retentionManager := backup.NewS3RetentionManager(s3Client, objectManagerTestBucket, logger)

	objectManager := backup.NewBackupObjectManager(uploader, retentionManager, logger)

	// Test concurrent uploads
	const numGoroutines = 5
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			key := fmt.Sprintf("concurrent-test-%d", index)
			content := fmt.Sprintf("concurrent content %d", index)
			reader := strings.NewReader(content)
			metadata := map[string]string{"type": "rdb", "index": fmt.Sprintf("%d", index)}
			replicationID := fmt.Sprintf("replication-%d", index)

			err := objectManager.UploadWithProgressAndRetention(ctx, reader, key, metadata, replicationID, 5, func(int64) {
				// Progress callback - no action needed for test
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		require.NoError(t, err)
	}

	// Verify all files were uploaded
	listResp, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(objectManagerTestBucket),
		Prefix: aws.String("concurrent-test-"),
	})
	require.NoError(t, err)

	assert.Equal(t, numGoroutines, len(listResp.Contents))
}
