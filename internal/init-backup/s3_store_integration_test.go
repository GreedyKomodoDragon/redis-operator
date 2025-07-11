package initbackup

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

const (
	integrationTestBucket    = "redis-backups-test"
	minioUsername            = "minioadmin"
	minioPassword            = "minioadmin"
	skipIntegrationTestMsg   = "Skipping integration test in short mode"
	terminateContainerErrMsg = "Failed to terminate MinIO container: %v"
	redisBackupsPrefix       = "redis-backups/"
	testRDBKey               = "redis-backups/repl1/dump-20241225-120000.rdb"
)

// setupMinIOContainer starts a MinIO testcontainer and returns connection details
func setupMinIOContainer(ctx context.Context, t *testing.T) (*minio.MinioContainer, string, *s3.Client) {
	minioContainer, err := minio.Run(ctx,
		"minio/minio:RELEASE.2024-01-16T16-07-38Z",
		minio.WithUsername(minioUsername),
		minio.WithPassword(minioPassword),
	)
	require.NoError(t, err)

	// Get connection details
	connectionString, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Add http:// prefix if not present
	endpoint := connectionString
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	// Create S3 client for setup operations
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			minioUsername,
			minioPassword,
			"",
		)),
	)
	require.NoError(t, err)

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	// Create test bucket
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(integrationTestBucket),
	})
	require.NoError(t, err)

	return minioContainer, endpoint, s3Client
}

// createTestS3Store creates an S3Store configured to use the test MinIO container
func createTestS3Store(t *testing.T, endpoint string) *S3Store {
	// Set environment variables for S3Store
	originalVars := map[string]string{
		"S3_BUCKET":             os.Getenv("S3_BUCKET"),
		"AWS_REGION":            os.Getenv("AWS_REGION"),
		"AWS_ENDPOINT_URL":      os.Getenv("AWS_ENDPOINT_URL"),
		"AWS_ACCESS_KEY_ID":     os.Getenv("AWS_ACCESS_KEY_ID"),
		"AWS_SECRET_ACCESS_KEY": os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	// Set test values
	t.Setenv("S3_BUCKET", integrationTestBucket)
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ENDPOINT_URL", endpoint)
	t.Setenv("AWS_ACCESS_KEY_ID", minioUsername)
	t.Setenv("AWS_SECRET_ACCESS_KEY", minioPassword)

	// Clean up environment variables after test
	t.Cleanup(func() {
		for key, value := range originalVars {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	store, err := NewS3Store(logger)
	require.NoError(t, err)

	return store
}

// uploadTestFile uploads a test file to the MinIO bucket for testing
func uploadTestFile(ctx context.Context, t *testing.T, s3Client *s3.Client, key, content string) {
	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(integrationTestBucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(content),
	})
	require.NoError(t, err)
}

func TestS3StoreIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()

	// Start MinIO container
	minioContainer, endpoint, s3Client := setupMinIOContainer(ctx, t)
	defer func() {
		if err := minioContainer.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	// Create S3Store
	store := createTestS3Store(t, endpoint)
	defer store.Close()

	t.Run("GetBucketName", func(t *testing.T) {
		bucketName := store.GetBucketName()
		assert.Equal(t, integrationTestBucket, bucketName)
	})

	t.Run("ListFiles_EmptyBucket", func(t *testing.T) {
		files, err := store.ListFiles(ctx, redisBackupsPrefix)
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("ListFiles_WithFiles", func(t *testing.T) {
		// Upload test files
		testFiles := []struct {
			key     string
			content string
		}{
			{testRDBKey, "rdb content 1"},
			{"redis-backups/repl1/appendonly-20241225-120000.aof", "aof content 1"},
			{"redis-backups/repl2/dump-20241225-130000.rdb", "rdb content 2"},
			{"other-files/not-included.txt", "other content"},
		}

		for _, tf := range testFiles {
			uploadTestFile(ctx, t, s3Client, tf.key, tf.content)
		}

		// List files with prefix
		files, err := store.ListFiles(ctx, redisBackupsPrefix)
		require.NoError(t, err)

		// Should only get files with the prefix
		assert.Len(t, files, 3)

		// Verify file properties
		var foundKeys []string
		for _, file := range files {
			foundKeys = append(foundKeys, file.Key)
			assert.True(t, strings.HasPrefix(file.Key, redisBackupsPrefix))
			assert.Greater(t, file.Size, int64(0))
			assert.False(t, file.LastModified.IsZero())
		}

		expectedKeys := []string{
			testRDBKey,
			"redis-backups/repl1/appendonly-20241225-120000.aof",
			"redis-backups/repl2/dump-20241225-130000.rdb",
		}

		for _, expected := range expectedKeys {
			assert.Contains(t, foundKeys, expected)
		}
	})

	t.Run("ListFiles_NoPrefix", func(t *testing.T) {
		// List all files (no prefix)
		files, err := store.ListFiles(ctx, "")
		require.NoError(t, err)

		// Should get all files including the "other-files" one
		assert.GreaterOrEqual(t, len(files), 4)

		var foundKeys []string
		for _, file := range files {
			foundKeys = append(foundKeys, file.Key)
		}

		assert.Contains(t, foundKeys, "other-files/not-included.txt")
	})

	t.Run("ListFiles_NonExistentPrefix", func(t *testing.T) {
		files, err := store.ListFiles(ctx, "non-existent-prefix/")
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	// t.Run("DownloadFile_NotImplemented", func(t *testing.T) {
	// 	data, err := store.D(ctx, testRDBKey)
	// 	assert.Error(t, err)
	// 	assert.Nil(t, data)
	// 	assert.Contains(t, err.Error(), "download functionality not implemented yet")
	// })
}

func TestS3StoreIntegrationErrorCases(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()

	// Start MinIO container
	minioContainer, endpoint, _ := setupMinIOContainer(ctx, t)
	defer func() {
		if err := minioContainer.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	t.Run("ListFiles_InvalidBucket", func(t *testing.T) {
		// Create store with non-existent bucket
		originalBucket := os.Getenv("S3_BUCKET")
		t.Setenv("S3_BUCKET", "non-existent-bucket")
		t.Setenv("AWS_ENDPOINT_URL", endpoint)
		t.Setenv("AWS_ACCESS_KEY_ID", minioUsername)
		t.Setenv("AWS_SECRET_ACCESS_KEY", minioPassword)

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		store, err := NewS3Store(logger)
		require.NoError(t, err)
		defer store.Close()

		// Restore original value
		if originalBucket != "" {
			t.Setenv("S3_BUCKET", originalBucket)
		}

		// This should fail because the bucket doesn't exist
		files, err := store.ListFiles(ctx, redisBackupsPrefix)
		assert.Error(t, err)
		assert.Nil(t, files)
	})

	t.Run("ListFiles_ContextTimeout", func(t *testing.T) {
		store := createTestS3Store(t, endpoint)
		defer store.Close()

		// Create a context that times out immediately
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()

		// Wait for context to timeout
		time.Sleep(10 * time.Millisecond)

		files, err := store.ListFiles(timeoutCtx, redisBackupsPrefix)
		assert.Error(t, err)
		assert.Nil(t, files)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})
}

func TestS3StoreEndToEndWithService(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()

	// Start MinIO container
	minioContainer, endpoint, s3Client := setupMinIOContainer(ctx, t)
	defer func() {
		if err := minioContainer.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	// Create S3Store
	store := createTestS3Store(t, endpoint)
	defer store.Close()

	// Upload realistic backup files
	testFiles := []struct {
		key     string
		content string
	}{
		// Latest backup set
		{"redis-backups/repl-latest/dump-20241225-120000.rdb", "latest rdb content"},
		{"redis-backups/repl-latest/appendonly-20241225-120000.aof", "latest aof content"},

		// Older backup set
		{"redis-backups/repl-older/dump-20241224-120000.rdb", "older rdb content"},
		{"redis-backups/repl-older/appendonly-20241224-120000.aof", "older aof content"},

		// Different replication ID, same timestamp as latest
		{"redis-backups/repl-other/dump-20241225-120000.rdb", "other rdb content"},
		{"redis-backups/repl-other/appendonly-20241225-120000.aof", "other aof content"},
	}

	// Upload files with realistic timestamps (latest first)
	for i, tf := range testFiles {
		uploadTestFile(ctx, t, s3Client, tf.key, tf.content)

		// Add a small delay to ensure different LastModified times
		if i < len(testFiles)-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Create service with the S3Store
	service := NewService(store)

	// Create temporary directory for test data
	dataDir := t.TempDir()

	// Run the service
	err := service.Run(dataDir)
	require.NoError(t, err)

	// The service should have discovered the backup files
	// We can't directly test the internal state, but we verified it runs without error
	// In a real scenario, the service would download and restore the files
}

func TestNewS3StoreConfigurationErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	t.Run("MissingBucket", func(t *testing.T) {
		// Clear bucket env var
		t.Setenv("S3_BUCKET", "")

		store, err := NewS3Store(logger)
		assert.Error(t, err)
		assert.Nil(t, store)
		assert.Contains(t, err.Error(), "S3_BUCKET environment variable is required")
	})

	t.Run("InvalidEndpoint", func(t *testing.T) {
		t.Setenv("S3_BUCKET", "test-bucket")
		t.Setenv("AWS_ENDPOINT_URL", "invalid-endpoint")
		t.Setenv("AWS_ACCESS_KEY_ID", "test")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

		// This should succeed in creating the store (connection is tested on first use)
		store, err := NewS3Store(logger)
		assert.NoError(t, err)
		assert.NotNil(t, store)
		defer store.Close()
	})
}
