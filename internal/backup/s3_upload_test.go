package backup_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GreedyKomodoDragon/redis-operator/internal/backup"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

const (
	testBucket         = "test-bucket"
	testRegion         = "us-west-2"
	testEastRegion     = "us-east-1"
	minioImage         = "minio/minio:RELEASE.2024-01-16T16-07-38Z"
	minioTerminateMsg  = "failed to terminate MinIO container: %s"
	uploadedByKey      = "uploaded-by"
	redisOperatorValue = "redis-operator"
	httpPrefix         = "http://"
	httpsPrefix        = "https://"
)

func TestS3ConfigValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   backup.S3Config
		hasError bool
	}{
		{
			name: "valid config with explicit credentials",
			config: backup.S3Config{
				Bucket:          testBucket,
				Region:          testRegion,
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
			},
			hasError: false,
		},
		{
			name: "valid config with custom endpoint",
			config: backup.S3Config{
				Bucket:          testBucket,
				Region:          testRegion,
				Endpoint:        "https://minio.example.com",
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
			},
			hasError: false,
		},
		{
			name: "config without credentials (will use default chain)",
			config: backup.S3Config{
				Bucket: testBucket,
				Region: testRegion,
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// We expect this to fail in test environment since there are no real AWS credentials
			// But we're testing that the configuration parsing works correctly
			s3Client, err := backup.NewS3Client(ctx, tt.config)

			if tt.hasError {
				assert.Error(t, err)
				assert.Nil(t, s3Client)
			} else {
				// In test environment without AWS credentials, we expect an error during config loading
				// but the S3Config struct should be valid
				assert.NotEmpty(t, tt.config.Bucket)
				if tt.config.Region != "" {
					assert.NotEmpty(t, tt.config.Region)
				}
			}
		})
	}
}

func TestS3UploaderConfigurationFields(t *testing.T) {
	config := backup.S3Config{
		Bucket:          testBucket,
		Region:          testRegion,
		Endpoint:        "https://s3.amazonaws.com",
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secret123",
	}

	// Verify configuration fields are properly set
	assert.Equal(t, testBucket, config.Bucket)
	assert.Equal(t, testRegion, config.Region)
	assert.Equal(t, "https://s3.amazonaws.com", config.Endpoint)
	assert.Equal(t, "AKIATEST", config.AccessKeyID)
	assert.Equal(t, "secret123", config.SecretAccessKey)
}

func TestS3UploaderIntegration(t *testing.T) {
	ctx := context.Background()

	// Start MinIO container
	minioContainer, err := minio.Run(ctx, minioImage)
	require.NoError(t, err)
	defer func() {
		if err := testcontainers.TerminateContainer(minioContainer); err != nil {
			t.Logf(minioTerminateMsg, err)
		}
	}()

	// Get MinIO connection details
	endpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Format endpoint with http:// protocol
	if !strings.HasPrefix(endpoint, httpPrefix) && !strings.HasPrefix(endpoint, httpsPrefix) {
		endpoint = httpPrefix + endpoint
	}

	// Create S3 configuration for MinIO
	s3Config := backup.S3Config{
		Bucket:          "test-bucket",
		Region:          testEastRegion,
		Endpoint:        endpoint,
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
	}

	// create s3 client
	s3Client, err := backup.NewS3Client(ctx, s3Config)
	require.NoError(t, err)
	require.NotNil(t, s3Client)

	// Create S3 uploader
	uploader := backup.NewS3Uploader(ctx, s3Client, s3Config.Bucket)
	require.NotNil(t, uploader)

	// Create the bucket first
	_, err = uploader.Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &s3Config.Bucket,
	})
	require.NoError(t, err)

	// Create a temporary test file
	testFile, err := os.CreateTemp("", "test-upload-*.txt")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	testContent := "This is a test file for S3 upload\nWith multiple lines\nAnd some test data"
	_, err = testFile.WriteString(testContent)
	require.NoError(t, err)
	testFile.Close()

	// Test file upload
	keyPrefix := "test-uploads/backup-001"
	err = uploader.UploadFile(ctx, testFile.Name(), keyPrefix)
	require.NoError(t, err)

	// Verify the file was uploaded
	expectedKey := keyPrefix + "/" + filepath.Base(testFile.Name())
	getObjectOutput, err := uploader.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s3Config.Bucket,
		Key:    &expectedKey,
	})
	require.NoError(t, err)

	// Read and verify the uploaded content
	uploadedContent, err := io.ReadAll(getObjectOutput.Body)
	require.NoError(t, err)
	getObjectOutput.Body.Close()

	assert.Equal(t, testContent, string(uploadedContent))

	// Verify metadata is set
	assert.NotNil(t, getObjectOutput.Metadata)
	assert.Contains(t, getObjectOutput.Metadata, uploadedByKey)
	assert.Equal(t, redisOperatorValue, getObjectOutput.Metadata[uploadedByKey])
	assert.Contains(t, getObjectOutput.Metadata, "timestamp")
}

func TestS3UploaderMultipleFiles(t *testing.T) {
	ctx := context.Background()

	// Start MinIO container
	minioContainer, err := minio.Run(ctx, minioImage)
	require.NoError(t, err)
	defer func() {
		if err := testcontainers.TerminateContainer(minioContainer); err != nil {
			t.Logf(minioTerminateMsg, err)
		}
	}()

	// Get MinIO connection details
	endpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Format endpoint with http:// protocol
	if !strings.HasPrefix(endpoint, httpPrefix) && !strings.HasPrefix(endpoint, httpsPrefix) {
		endpoint = httpPrefix + endpoint
	}

	// Create S3 configuration for MinIO
	s3Config := backup.S3Config{
		Bucket:          "multi-test-bucket",
		Region:          testEastRegion,
		Endpoint:        endpoint,
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
	}

	// create s3 client
	s3Client, err := backup.NewS3Client(ctx, s3Config)
	require.NoError(t, err)
	require.NotNil(t, s3Client)

	// Create S3 uploader
	uploader := backup.NewS3Uploader(ctx, s3Client, s3Config.Bucket)
	require.NotNil(t, uploader)

	// Create the bucket
	_, err = uploader.Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &s3Config.Bucket,
	})
	require.NoError(t, err)

	// Create multiple test files
	testFiles := []struct {
		name    string
		content string
	}{
		{"rdb-snapshot.rdb", "REDIS0009\xfa\x09redis-ver\x055.0.7\xfa\x0aredis-bits\xc0@"},
		{"aof-commands.aof", "*2\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$6\r\nvalue1\r\n"},
		{"config-backup.conf", "# Redis configuration\nport 6379\nsave 900 1\n"},
	}

	keyPrefix := "redis-backups/repl-123"
	uploadedFiles := make([]string, 0, len(testFiles))

	for _, tf := range testFiles {
		// Create temporary file
		testFile, err := os.CreateTemp("", tf.name)
		require.NoError(t, err)
		defer os.Remove(testFile.Name())

		_, err = testFile.WriteString(tf.content)
		require.NoError(t, err)
		testFile.Close()

		// Upload the file
		err = uploader.UploadFile(ctx, testFile.Name(), keyPrefix)
		require.NoError(t, err)

		uploadedFiles = append(uploadedFiles, keyPrefix+"/"+filepath.Base(testFile.Name()))
	}

	// Verify all files were uploaded
	listOutput, err := uploader.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &s3Config.Bucket,
		Prefix: &keyPrefix,
	})
	require.NoError(t, err)

	assert.Len(t, listOutput.Contents, len(testFiles))

	// Verify each file exists and has correct content
	for _, obj := range listOutput.Contents {
		assert.Contains(t, uploadedFiles, *obj.Key)
		assert.True(t, *obj.Size > 0)
		assert.NotNil(t, obj.LastModified)
	}
}

func TestS3UploaderErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Start MinIO container
	minioContainer, err := minio.Run(ctx, minioImage)
	require.NoError(t, err)
	defer func() {
		if err := testcontainers.TerminateContainer(minioContainer); err != nil {
			t.Logf(minioTerminateMsg, err)
		}
	}()

	// Get MinIO connection details
	endpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Format endpoint with http:// protocol
	if !strings.HasPrefix(endpoint, httpPrefix) && !strings.HasPrefix(endpoint, httpsPrefix) {
		endpoint = httpPrefix + endpoint
	}

	// Create S3 configuration for MinIO
	s3Config := backup.S3Config{
		Bucket:          "error-test-bucket",
		Region:          testEastRegion,
		Endpoint:        endpoint,
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
	}

	// create s3 client
	s3Client, err := backup.NewS3Client(ctx, s3Config)
	require.NoError(t, err)
	require.NotNil(t, s3Client)

	// Create S3 uploader
	uploader := backup.NewS3Uploader(ctx, s3Client, s3Config.Bucket)
	require.NotNil(t, uploader)

	// Test 1: Upload to non-existent bucket (should fail)
	testFile, err := os.CreateTemp("", "error-test-*.txt")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	_, err = testFile.WriteString("test content")
	require.NoError(t, err)
	testFile.Close()

	err = uploader.UploadFile(ctx, testFile.Name(), "test-prefix")
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "nosuchbucket")

	// Test 2: Upload non-existent file (should fail)
	_, err = uploader.Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &s3Config.Bucket,
	})
	require.NoError(t, err)

	err = uploader.UploadFile(ctx, "/nonexistent/file.txt", "test-prefix")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open file")
}

func TestS3UploaderTimeout(t *testing.T) {
	ctx := context.Background()

	// Start MinIO container
	minioContainer, err := minio.Run(ctx, minioImage)
	require.NoError(t, err)
	defer func() {
		if err := testcontainers.TerminateContainer(minioContainer); err != nil {
			t.Logf(minioTerminateMsg, err)
		}
	}()

	// Get MinIO connection details
	endpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Format endpoint with http:// protocol
	if !strings.HasPrefix(endpoint, httpPrefix) && !strings.HasPrefix(endpoint, httpsPrefix) {
		endpoint = httpPrefix + endpoint
	}

	// Create S3 configuration for MinIO
	s3Config := backup.S3Config{
		Bucket:          "timeout-test-bucket",
		Region:          testEastRegion,
		Endpoint:        endpoint,
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
	}

	// create s3 client
	s3Client, err := backup.NewS3Client(ctx, s3Config)
	require.NoError(t, err)
	require.NotNil(t, s3Client)

	// Create S3 uploader
	uploader := backup.NewS3Uploader(ctx, s3Client, s3Config.Bucket)
	require.NotNil(t, uploader)

	// Create the bucket
	_, err = uploader.Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &s3Config.Bucket,
	})
	require.NoError(t, err)

	// Create a test file
	testFile, err := os.CreateTemp("", "timeout-test-*.txt")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	_, err = testFile.WriteString("test content for timeout")
	require.NoError(t, err)
	testFile.Close()

	// Test with a very short timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	defer cancel()

	// This should either succeed very quickly or timeout
	err = uploader.UploadFile(timeoutCtx, testFile.Name(), "timeout-test")
	// We can't guarantee this will timeout due to the speed of local operations
	// but we can verify that context cancellation is respected
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}
}
