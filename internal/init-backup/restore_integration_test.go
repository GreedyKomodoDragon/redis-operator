package initbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

const (
	testRestoreBucket = "redis-restore-test"
)

// Test data for RDB and AOF files
var (
	testRDBContent = []byte("REDIS0011\xfa\tredis-ver\x057.2.0\xfa\nredis-bits\xc0@\xfa\x05ctime\xc2\xe8\x8c\xd7f\xfa\bused-mem\xc2`\x0e\x00\x00\xfa\taof-base\xc0\x00\xff\x12\xd4\xed\x00\x00\x00\x00")
	testAOFContent = []byte("*2\r\n$6\r\nSELECT\r\n$1\r\n0\r\n*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n")
)

func TestRestoreBackupSetIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()
	container, endpoint, s3Client := setupMinIOForRestore(ctx, t)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	// Setup test data
	setupTestBackupFiles(ctx, t, s3Client)

	// Create S3Store with the test endpoint
	s3Store := createRestoreTestS3Store(t, endpoint)
	defer s3Store.Close()

	// Create service and test restoration
	service := NewService(s3Store)

	// Create temporary data directory
	dataDir := t.TempDir()

	// Override the data directory for testing
	err := testRestoreBackupSetWithDataDir(ctx, t, service, dataDir)
	require.NoError(t, err)

	// Verify restored files
	verifyRestoredFiles(t, dataDir)
}

func TestRestoreRDBFileOnly(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()
	container, endpoint, s3Client := setupMinIOForRestore(ctx, t)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	// Setup only RDB file
	rdbKey := "redis-backups/repl123/dump-20240101-000000.rdb"
	putTestObject(ctx, t, s3Client, rdbKey, testRDBContent)

	// Create S3Store and service
	s3Store := createRestoreTestS3Store(t, endpoint)
	defer s3Store.Close()
	service := NewService(s3Store)

	// Create temporary data directory
	dataDir := t.TempDir()

	// Test restoration
	err := testRestoreBackupSetWithDataDir(ctx, t, service, dataDir)
	require.NoError(t, err)

	// Verify only RDB file is restored
	verifyRDBFile(t, dataDir)
	verifyNoAOFFiles(t, dataDir)
}

func TestRestoreWithMultipleAOFFiles(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()
	container, endpoint, s3Client := setupMinIOForRestore(ctx, t)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	// Setup RDB and multiple AOF files with same timestamp
	replID := "repl456"
	timestamp := "20240101-000000"

	rdbKey := fmt.Sprintf("redis-backups/%s/dump-%s.rdb", replID, timestamp)
	aof1Key := fmt.Sprintf("redis-backups/%s/appendonly-%s.aof", replID, timestamp)
	// Create a second AOF with slightly different timestamp to simulate multiple files
	timestamp2 := "20240101-000001"
	aof2Key := fmt.Sprintf("redis-backups/%s/appendonly-%s.aof", replID, timestamp2)

	putTestObject(ctx, t, s3Client, rdbKey, testRDBContent)
	putTestObject(ctx, t, s3Client, aof1Key, testAOFContent)
	putTestObject(ctx, t, s3Client, aof2Key, append(testAOFContent, []byte("*3\r\n$3\r\nSET\r\n$4\r\nkey2\r\n$6\r\nvalue2\r\n")...))

	// Create S3Store and service
	s3Store := createRestoreTestS3Store(t, endpoint)
	defer s3Store.Close()
	service := NewService(s3Store)

	// Create temporary data directory
	dataDir := t.TempDir()

	// Test restoration
	err := testRestoreBackupSetWithDataDir(ctx, t, service, dataDir)
	require.NoError(t, err)

	// Verify RDB and single matching AOF file (only one should match the RDB timestamp)
	verifyRDBFile(t, dataDir)
	verifySingleAOFFile(t, dataDir)
}

func TestRestoreWithLargeFiles(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()
	container, endpoint, s3Client := setupMinIOForRestore(ctx, t)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	// Create a large RDB file (10MB) to test streaming
	largeRDBContent := make([]byte, 10*1024*1024)
	copy(largeRDBContent[:len(testRDBContent)], testRDBContent)
	for i := len(testRDBContent); i < len(largeRDBContent); i++ {
		largeRDBContent[i] = byte(i % 256)
	}

	rdbKey := "redis-backups/repl789/dump-20240101-000000.rdb"
	putTestObject(ctx, t, s3Client, rdbKey, largeRDBContent)

	// Create S3Store and service
	s3Store := createRestoreTestS3Store(t, endpoint)
	defer s3Store.Close()
	service := NewService(s3Store)

	// Create temporary data directory
	dataDir := t.TempDir()

	// Test restoration
	err := testRestoreBackupSetWithDataDir(ctx, t, service, dataDir)
	require.NoError(t, err)

	// Verify large file was restored correctly
	rdbPath := filepath.Join(dataDir, "dump.rdb")
	restoredContent, err := os.ReadFile(rdbPath)
	require.NoError(t, err)
	assert.Equal(t, len(largeRDBContent), len(restoredContent))
	assert.Equal(t, largeRDBContent[:100], restoredContent[:100]) // Check first 100 bytes
}

func TestRestoreErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip(skipIntegrationTestMsg)
	}

	ctx := context.Background()
	container, endpoint, s3Client := setupMinIOForRestore(ctx, t)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf(terminateContainerErrMsg, err)
		}
	}()

	// Setup valid RDB file
	rdbKey := "redis-backups/repl999/dump-20240101-000000.rdb"
	putTestObject(ctx, t, s3Client, rdbKey, testRDBContent)

	// Create S3Store and service
	s3Store := createRestoreTestS3Store(t, endpoint)
	defer s3Store.Close()
	service := NewService(s3Store)

	// Test with invalid data directory (read-only)
	invalidDataDir := "/proc/invalid-dir"
	err := testRestoreBackupSetWithDataDir(ctx, t, service, invalidDataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create data directory")
}

// Helper functions

func setupMinIOForRestore(ctx context.Context, t *testing.T) (*minio.MinioContainer, string, *s3.Client) {
	container, err := minio.Run(ctx,
		"minio/minio:RELEASE.2024-01-16T16-07-38Z",
		minio.WithUsername(minioUsername),
		minio.WithPassword(minioPassword),
	)
	require.NoError(t, err)

	connectionString, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	endpoint := connectionString
	if !strings.HasPrefix(endpoint, "http://") {
		endpoint = "http://" + endpoint
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			minioUsername, minioPassword, "",
		)),
	)
	require.NoError(t, err)

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	// Create bucket
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(testRestoreBucket),
	})
	require.NoError(t, err)

	return container, endpoint, s3Client
}

func setupTestBackupFiles(ctx context.Context, t *testing.T, s3Client *s3.Client) {
	replID := "repl123"
	timestamp := "20240101-000000"

	rdbKey := fmt.Sprintf("redis-backups/%s/dump-%s.rdb", replID, timestamp)
	aofKey := fmt.Sprintf("redis-backups/%s/appendonly-%s.aof", replID, timestamp)

	putTestObject(ctx, t, s3Client, rdbKey, testRDBContent)
	putTestObject(ctx, t, s3Client, aofKey, testAOFContent)
}

func putTestObject(ctx context.Context, t *testing.T, s3Client *s3.Client, key string, content []byte) {
	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(testRestoreBucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(string(content)),
	})
	require.NoError(t, err)
}

func createRestoreTestS3Store(t *testing.T, endpoint string) *S3Store {
	// Set environment variables for S3Store
	os.Setenv("S3_BUCKET", testRestoreBucket)
	os.Setenv("AWS_REGION", "us-east-1")
	os.Setenv("AWS_ENDPOINT_URL", endpoint)
	os.Setenv("AWS_ACCESS_KEY_ID", minioUsername)
	os.Setenv("AWS_SECRET_ACCESS_KEY", minioPassword)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	s3Store, err := NewS3Store(logger)
	require.NoError(t, err)
	return s3Store
}

func testRestoreBackupSetWithDataDir(ctx context.Context, t *testing.T, service *Service, dataDir string) error {
	return service.Run(dataDir)
}

func verifyRestoredFiles(t *testing.T, dataDir string) {
	verifyRDBFile(t, dataDir)
	verifySingleAOFFile(t, dataDir)
}

func verifyRDBFile(t *testing.T, dataDir string) {
	rdbPath := filepath.Join(dataDir, "dump.rdb")
	assert.FileExists(t, rdbPath)

	content, err := os.ReadFile(rdbPath)
	require.NoError(t, err)
	assert.Equal(t, testRDBContent, content)
}

func verifySingleAOFFile(t *testing.T, dataDir string) {
	aofPath := filepath.Join(dataDir, "appendonly.aof")
	assert.FileExists(t, aofPath)

	content, err := os.ReadFile(aofPath)
	require.NoError(t, err)
	assert.Equal(t, testAOFContent, content)
}

func verifyNoAOFFiles(t *testing.T, dataDir string) {
	aofPath := filepath.Join(dataDir, "appendonly.aof")
	assert.NoFileExists(t, aofPath)
}
