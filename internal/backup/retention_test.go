package backup_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GreedyKomodoDragon/redis-operator/internal/backup"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

const (
	retentionTestBucket        = "retention-test-bucket"
	retentionMinioImage        = "minio/minio:RELEASE.2024-01-16T16-07-38Z"
	retentionMinioTerminateMsg = "failed to terminate MinIO container: %s"
	retentionHTTPPrefix        = "http://"
	retentionHTTPSPrefix       = "https://"

	// Test data constants
	testRDBContent1 = "rdb content 1"
	testRDBContent2 = "rdb content 2"
	testRDBContent3 = "rdb content 3"
	testRDBContent4 = "rdb content 4"
	testRDBContent5 = "rdb content 5"
	testAOFContent1 = "aof content 1"
	testAOFContent2 = "aof content 2"
	testAOFContent3 = "aof content 3"

	// File path patterns
	rdbFilePattern = "redis-backups/%s/dump-%s.rdb"
	aofFilePattern = "redis-backups/%s/appendonly-%s.aof"
)

// setupMinIOForRetention sets up MinIO container and returns S3 client and bucket
func setupMinIOForRetention(t *testing.T) (context.Context, *s3.Client, func()) {
	ctx := context.Background()

	// Start MinIO container
	minioContainer, err := minio.Run(ctx, retentionMinioImage)
	require.NoError(t, err)

	// Get MinIO connection details
	endpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Format endpoint with http:// protocol
	if !strings.HasPrefix(endpoint, retentionHTTPPrefix) && !strings.HasPrefix(endpoint, retentionHTTPSPrefix) {
		endpoint = retentionHTTPPrefix + endpoint
	}

	// Create S3 configuration for MinIO
	s3Config := backup.S3Config{
		Bucket:          retentionTestBucket,
		Region:          "us-east-1",
		Endpoint:        endpoint,
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
	}

	// Create S3 client
	s3Client, err := backup.NewS3Client(ctx, s3Config)
	require.NoError(t, err)

	// Create bucket
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(retentionTestBucket),
	})
	require.NoError(t, err)

	cleanup := func() {
		if err := testcontainers.TerminateContainer(minioContainer); err != nil {
			t.Logf(retentionMinioTerminateMsg, err)
		}
	}

	return ctx, s3Client, cleanup
}

// uploadTestBackupFile uploads a test backup file to MinIO
func uploadTestBackupFile(ctx context.Context, t *testing.T, s3Client *s3.Client, key, content string) {
	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(retentionTestBucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(content),
	})
	require.NoError(t, err)
}

// TestS3RetentionManagerManageRetention tests the basic retention management functionality
func TestS3RetentionManagerManageRetention(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create retention manager
	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-replication-id"

	// Upload test backup files with different timestamps
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf(rdbFilePattern, replID, "20240101-120000"), testRDBContent1},
		{fmt.Sprintf(rdbFilePattern, replID, "20240102-120000"), testRDBContent2},
		{fmt.Sprintf(rdbFilePattern, replID, "20240103-120000"), testRDBContent3},
		{fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"), testRDBContent4},
		{fmt.Sprintf(rdbFilePattern, replID, "20240105-120000"), testRDBContent5},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		// Add small delay to ensure different LastModified times
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 3 files
	err := retentionManager.ManageRetention(ctx, replID, 3)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 3 files remaining
	assert.Len(t, files, 3)

	// Verify the correct files remain (the 3 newest)
	expectedKeys := []string{
		fmt.Sprintf(rdbFilePattern, replID, "20240103-120000"),
		fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"),
		fmt.Sprintf(rdbFilePattern, replID, "20240105-120000"),
	}

	actualKeys := make([]string, len(files))
	for i, file := range files {
		actualKeys[i] = file.Key
	}

	for _, expectedKey := range expectedKeys {
		assert.Contains(t, actualKeys, expectedKey)
	}
}

// TestS3RetentionManagerManageRetentionWithAOF tests retention with both RDB and AOF files
func TestS3RetentionManagerManageRetentionWithAOF(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-replication-id-aof"

	// Upload test backup files with RDB and AOF pairs
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/appendonly-20240101-120000.aof", replID), "aof content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID), "rdb content 2"},
		{fmt.Sprintf("redis-backups/%s/appendonly-20240102-120000.aof", replID), "aof content 2"},
		{fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID), "rdb content 3"},
		{fmt.Sprintf("redis-backups/%s/appendonly-20240103-120000.aof", replID), "aof content 3"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 2 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 2)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 4 files remaining (2 RDB + 2 AOF)
	assert.Len(t, files, 4)

	// Verify the correct files remain (the 2 newest backup sets)
	expectedKeys := []string{
		fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID),
		fmt.Sprintf("redis-backups/%s/appendonly-20240102-120000.aof", replID),
		fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID),
		fmt.Sprintf("redis-backups/%s/appendonly-20240103-120000.aof", replID),
	}

	actualKeys := make([]string, len(files))
	for i, file := range files {
		actualKeys[i] = file.Key
	}

	for _, expectedKey := range expectedKeys {
		assert.Contains(t, actualKeys, expectedKey)
	}
}

// TestS3RetentionManagerGetFilesToDelete tests the file deletion logic
func TestS3RetentionManagerGetFilesToDelete(t *testing.T) {
	_, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	// Create test backup files
	testFiles := []backup.BackupFile{
		{
			Key:          "redis-backups/test-repl/dump-20240101-120000.rdb",
			LastModified: time.Now().Add(-4 * time.Hour),
			Size:         1024,
		},
		{
			Key:          "redis-backups/test-repl/dump-20240102-120000.rdb",
			LastModified: time.Now().Add(-3 * time.Hour),
			Size:         2048,
		},
		{
			Key:          "redis-backups/test-repl/dump-20240103-120000.rdb",
			LastModified: time.Now().Add(-2 * time.Hour),
			Size:         3072,
		},
		{
			Key:          "redis-backups/test-repl/dump-20240104-120000.rdb",
			LastModified: time.Now().Add(-1 * time.Hour),
			Size:         4096,
		},
	}

	// Test with retention count of 2
	filesToDelete, err := retentionManager.GetFilesToDelete(testFiles, 2)
	require.NoError(t, err)

	// Should delete 2 oldest files
	assert.Len(t, filesToDelete, 2)

	// Sort the files to ensure deterministic testing
	sort.Slice(filesToDelete, func(i, j int) bool {
		return filesToDelete[i].Key < filesToDelete[j].Key
	})

	assert.Equal(t, "redis-backups/test-repl/dump-20240101-120000.rdb", filesToDelete[0].Key)
	assert.Equal(t, "redis-backups/test-repl/dump-20240102-120000.rdb", filesToDelete[1].Key)

	// Test with retention count equal to number of files
	filesToDelete, err = retentionManager.GetFilesToDelete(testFiles, 4)
	require.NoError(t, err)
	assert.Len(t, filesToDelete, 0)

	// Test with retention count greater than number of files
	filesToDelete, err = retentionManager.GetFilesToDelete(testFiles, 10)
	require.NoError(t, err)
	assert.Len(t, filesToDelete, 0)

	// Test with retention count of 0 (disabled)
	filesToDelete, err = retentionManager.GetFilesToDelete(testFiles, 0)
	require.NoError(t, err)
	assert.Len(t, filesToDelete, 0)
}

// TestS3RetentionManagerListBackupFiles tests the file listing functionality
func TestS3RetentionManagerListBackupFiles(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-list-replication-id"

	// Upload test backup files
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/appendonly-20240101-120000.aof", replID), "aof content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID), "rdb content 2"},
		// Add a file from different replication ID that should not be listed
		{"redis-backups/other-repl/dump-20240101-120000.rdb", "other rdb content"},
		// Add a file not following the backup pattern
		{fmt.Sprintf("redis-backups/%s/config.txt", replID), "config content"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
	}

	// List backup files for the specific replication ID
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 3 files (2 RDB + 1 AOF + 1 config)
	assert.Len(t, files, 4)

	// Verify file properties
	for _, file := range files {
		assert.True(t, strings.HasPrefix(file.Key, fmt.Sprintf("redis-backups/%s/", replID)))
		assert.True(t, file.Size > 0)
		assert.False(t, file.LastModified.IsZero())
	}
}

// TestS3RetentionManagerDeleteBackupFiles tests the file deletion functionality
func TestS3RetentionManagerDeleteBackupFiles(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-delete-replication-id"

	// Upload test backup files
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID), "rdb content 2"},
		{fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID), "rdb content 3"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
	}

	// Verify files exist
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)
	assert.Len(t, files, 3)

	// Delete specific files
	keysToDelete := []string{
		fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID),
		fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID),
	}

	err = retentionManager.DeleteBackupFiles(ctx, keysToDelete)
	require.NoError(t, err)

	// Verify files were deleted
	files, err = retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID), files[0].Key)
}

// TestS3RetentionManagerRetentionDisabled tests behavior when retention is disabled
func TestS3RetentionManagerRetentionDisabled(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-disabled-replication-id"

	// Upload test backup files
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID), "rdb content 2"},
		{fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID), "rdb content 3"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
	}

	// Test with retention disabled (retention count = 0)
	err := retentionManager.ManageRetention(ctx, replID, 0)
	require.NoError(t, err)

	// All files should still exist
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)
	assert.Len(t, files, 3)

	// Test with negative retention count
	err = retentionManager.ManageRetention(ctx, replID, -1)
	require.NoError(t, err)

	// All files should still exist
	files, err = retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)
	assert.Len(t, files, 3)
}

// TestS3RetentionManagerGroupBackupsByTimestamp tests the timestamp grouping logic
func TestS3RetentionManagerGroupBackupsByTimestamp(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-group-replication-id"

	// Upload test backup files with same timestamp (backup set)
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/appendonly-20240101-120000.aof", replID), "aof content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID), "rdb content 2"},
		{fmt.Sprintf("redis-backups/%s/appendonly-20240102-120000.aof", replID), "aof content 2"},
		{fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID), "rdb content 3"},
		{fmt.Sprintf("redis-backups/%s/appendonly-20240103-120000.aof", replID), "aof content 3"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 2 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 2)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 4 files remaining (2 backup sets * 2 files each)
	assert.Len(t, files, 4)

	// Verify that files from the oldest backup set (20240101) are deleted
	for _, file := range files {
		assert.NotContains(t, file.Key, "20240101")
	}
}

// TestS3RetentionManagerInvalidTimestampHandling tests handling of files with invalid timestamps
func TestS3RetentionManagerInvalidTimestampHandling(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-invalid-timestamp-replication-id"

	// Upload test backup files with valid and invalid timestamps
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID), "rdb content 2"},
		{fmt.Sprintf("redis-backups/%s/dump-invalid-timestamp.rdb", replID), "invalid rdb content"},
		{fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID), "rdb content 3"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 2 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 2)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 3 files remaining (2 valid + 1 invalid that wasn't processed)
	assert.Len(t, files, 3)

	// Verify that the invalid timestamp file still exists
	invalidFileExists := false
	for _, file := range files {
		if strings.Contains(file.Key, "invalid-timestamp") {
			invalidFileExists = true
			break
		}
	}
	assert.True(t, invalidFileExists, "File with invalid timestamp should still exist")
}

// TestS3RetentionManagerEmptyBucket tests behavior with empty bucket
func TestS3RetentionManagerEmptyBucket(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-empty-replication-id"

	// Test retention on empty bucket
	err := retentionManager.ManageRetention(ctx, replID, 5)
	require.NoError(t, err)

	// List files should return empty
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)
	assert.Len(t, files, 0)
}

// TestS3RetentionManagerConcurrentOperations tests concurrent retention operations
func TestS3RetentionManagerConcurrentOperations(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID1 := "test-concurrent-replication-id-1"
	replID2 := "test-concurrent-replication-id-2"

	// Upload test backup files for both replication IDs
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID1), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID1), "rdb content 2"},
		{fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID1), "rdb content 3"},
		{fmt.Sprintf("redis-backups/%s/dump-20240101-120000.rdb", replID2), "rdb content 1"},
		{fmt.Sprintf("redis-backups/%s/dump-20240102-120000.rdb", replID2), "rdb content 2"},
		{fmt.Sprintf("redis-backups/%s/dump-20240103-120000.rdb", replID2), "rdb content 3"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Run retention management concurrently for both replication IDs
	done := make(chan error, 2)

	go func() {
		done <- retentionManager.ManageRetention(ctx, replID1, 2)
	}()

	go func() {
		done <- retentionManager.ManageRetention(ctx, replID2, 1)
	}()

	// Wait for both operations to complete
	for i := 0; i < 2; i++ {
		err := <-done
		require.NoError(t, err)
	}

	// Verify results
	files1, err := retentionManager.ListBackupFiles(ctx, replID1)
	require.NoError(t, err)
	assert.Len(t, files1, 2) // Should keep 2 files

	files2, err := retentionManager.ListBackupFiles(ctx, replID2)
	require.NoError(t, err)
	assert.Len(t, files2, 1) // Should keep 1 file
}

// TestS3RetentionManagerErrorHandling tests error handling scenarios
func TestS3RetentionManagerErrorHandling(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Test with non-existent bucket
	retentionManager := backup.NewS3RetentionManager(s3Client, "non-existent-bucket", logger)

	replID := "test-error-replication-id"

	// This should return an error
	err := retentionManager.ManageRetention(ctx, replID, 5)
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "failed to list backup files")

	// Test deleting non-existent files
	retentionManager = backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)
	err = retentionManager.DeleteBackupFiles(ctx, []string{"non-existent-key"})
	// This should not error as S3 delete operations are idempotent
	assert.NoError(t, err)
}

// TestS3RetentionManagerMixedFileTypesGetFilesToDelete tests retention logic with mixed RDB and AOF files
func TestS3RetentionManagerMixedFileTypesGetFilesToDelete(t *testing.T) {
	_, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	// Create mixed backup files with different timestamps
	testFiles := []backup.BackupFile{
		// Backup set 1 (oldest)
		{
			Key:          "redis-backups/test-mixed/dump-20240101-120000.rdb",
			LastModified: time.Now().Add(-8 * time.Hour),
			Size:         1024,
		},
		{
			Key:          "redis-backups/test-mixed/appendonly-20240101-120000.aof",
			LastModified: time.Now().Add(-8 * time.Hour),
			Size:         512,
		},
		// Backup set 2
		{
			Key:          "redis-backups/test-mixed/dump-20240102-120000.rdb",
			LastModified: time.Now().Add(-6 * time.Hour),
			Size:         2048,
		},
		{
			Key:          "redis-backups/test-mixed/appendonly-20240102-120000.aof",
			LastModified: time.Now().Add(-6 * time.Hour),
			Size:         1024,
		},
		// Backup set 3
		{
			Key:          "redis-backups/test-mixed/dump-20240103-120000.rdb",
			LastModified: time.Now().Add(-4 * time.Hour),
			Size:         3072,
		},
		{
			Key:          "redis-backups/test-mixed/appendonly-20240103-120000.aof",
			LastModified: time.Now().Add(-4 * time.Hour),
			Size:         1536,
		},
		// Backup set 4 (newest)
		{
			Key:          "redis-backups/test-mixed/dump-20240104-120000.rdb",
			LastModified: time.Now().Add(-2 * time.Hour),
			Size:         4096,
		},
		{
			Key:          "redis-backups/test-mixed/appendonly-20240104-120000.aof",
			LastModified: time.Now().Add(-2 * time.Hour),
			Size:         2048,
		},
	}

	// Test with retention count of 2 (keep 2 backup sets)
	filesToDelete, err := retentionManager.GetFilesToDelete(testFiles, 2)
	require.NoError(t, err)

	// Should delete 4 files (2 oldest backup sets * 2 files each)
	assert.Len(t, filesToDelete, 4)

	// Sort the files to ensure deterministic testing
	sort.Slice(filesToDelete, func(i, j int) bool {
		return filesToDelete[i].Key < filesToDelete[j].Key
	})

	// Verify that files from the 2 oldest backup sets are marked for deletion
	expectedDeletedKeys := []string{
		"redis-backups/test-mixed/appendonly-20240101-120000.aof",
		"redis-backups/test-mixed/appendonly-20240102-120000.aof",
		"redis-backups/test-mixed/dump-20240101-120000.rdb",
		"redis-backups/test-mixed/dump-20240102-120000.rdb",
	}

	actualDeletedKeys := make([]string, len(filesToDelete))
	for i, file := range filesToDelete {
		actualDeletedKeys[i] = file.Key
	}

	assert.ElementsMatch(t, expectedDeletedKeys, actualDeletedKeys)

	// Test with retention count of 3 (keep 3 backup sets)
	filesToDelete, err = retentionManager.GetFilesToDelete(testFiles, 3)
	require.NoError(t, err)

	// Should delete 2 files (1 oldest backup set * 2 files)
	assert.Len(t, filesToDelete, 2)

	// Sort the files to ensure deterministic testing
	sort.Slice(filesToDelete, func(i, j int) bool {
		return filesToDelete[i].Key < filesToDelete[j].Key
	})

	// Verify that only the oldest backup set is marked for deletion
	expectedDeletedKeys = []string{
		"redis-backups/test-mixed/appendonly-20240101-120000.aof",
		"redis-backups/test-mixed/dump-20240101-120000.rdb",
	}

	actualDeletedKeys = make([]string, len(filesToDelete))
	for i, file := range filesToDelete {
		actualDeletedKeys[i] = file.Key
	}

	assert.ElementsMatch(t, expectedDeletedKeys, actualDeletedKeys)
}

// TestS3RetentionManagerMixedFileTypesManageRetention tests full retention management with mixed file types
func TestS3RetentionManagerMixedFileTypesManageRetention(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-mixed-retention-replication-id"

	// Upload test backup files with mixed RDB and AOF files
	testFiles := []struct {
		key     string
		content string
	}{
		// Backup set 1 (oldest)
		{fmt.Sprintf(rdbFilePattern, replID, "20240101-120000"), testRDBContent1},
		{fmt.Sprintf(aofFilePattern, replID, "20240101-120000"), testAOFContent1},
		// Backup set 2
		{fmt.Sprintf(rdbFilePattern, replID, "20240102-120000"), testRDBContent2},
		{fmt.Sprintf(aofFilePattern, replID, "20240102-120000"), testAOFContent2},
		// Backup set 3
		{fmt.Sprintf(rdbFilePattern, replID, "20240103-120000"), testRDBContent3},
		{fmt.Sprintf(aofFilePattern, replID, "20240103-120000"), testAOFContent3},
		// Backup set 4 (newest)
		{fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"), testRDBContent4},
		{fmt.Sprintf(aofFilePattern, replID, "20240104-120000"), "aof content 4"},
		// Backup set 5 (newest)
		{fmt.Sprintf(rdbFilePattern, replID, "20240105-120000"), testRDBContent5},
		{fmt.Sprintf(aofFilePattern, replID, "20240105-120000"), "aof content 5"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 3 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 3)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 6 files remaining (3 backup sets * 2 files each)
	assert.Len(t, files, 6)

	// Verify the correct files remain (the 3 newest backup sets)
	expectedRemainingKeys := []string{
		fmt.Sprintf(rdbFilePattern, replID, "20240103-120000"),
		fmt.Sprintf(aofFilePattern, replID, "20240103-120000"),
		fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"),
		fmt.Sprintf(aofFilePattern, replID, "20240104-120000"),
		fmt.Sprintf(rdbFilePattern, replID, "20240105-120000"),
		fmt.Sprintf(aofFilePattern, replID, "20240105-120000"),
	}

	actualRemainingKeys := make([]string, len(files))
	for i, file := range files {
		actualRemainingKeys[i] = file.Key
	}

	assert.ElementsMatch(t, expectedRemainingKeys, actualRemainingKeys)

	// Verify that the deleted files are actually gone
	deletedKeys := []string{
		fmt.Sprintf(rdbFilePattern, replID, "20240101-120000"),
		fmt.Sprintf(aofFilePattern, replID, "20240101-120000"),
		fmt.Sprintf(rdbFilePattern, replID, "20240102-120000"),
		fmt.Sprintf(aofFilePattern, replID, "20240102-120000"),
	}

	for _, key := range deletedKeys {
		assert.NotContains(t, actualRemainingKeys, key)
	}
}

// TestS3RetentionManagerMixedFileTypesIncompleteBackupSets tests handling of incomplete backup sets
func TestS3RetentionManagerMixedFileTypesIncompleteBackupSets(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-incomplete-backup-sets-replication-id"

	// Upload test backup files with some incomplete backup sets
	testFiles := []struct {
		key     string
		content string
	}{
		// Complete backup set 1
		{fmt.Sprintf(rdbFilePattern, replID, "20240101-120000"), testRDBContent1},
		{fmt.Sprintf(aofFilePattern, replID, "20240101-120000"), testAOFContent1},
		// Incomplete backup set 2 (only RDB)
		{fmt.Sprintf(rdbFilePattern, replID, "20240102-120000"), testRDBContent2},
		// Incomplete backup set 3 (only AOF)
		{fmt.Sprintf(aofFilePattern, replID, "20240103-120000"), testAOFContent3},
		// Complete backup set 4
		{fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"), testRDBContent4},
		{fmt.Sprintf(aofFilePattern, replID, "20240104-120000"), "aof content 4"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 2 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 2)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 3 files remaining (the 2 newest backup sets by timestamp)
	assert.Len(t, files, 3)

	// Verify the correct files remain (should keep the 2 newest timestamp groups)
	expectedRemainingKeys := []string{
		fmt.Sprintf(aofFilePattern, replID, "20240103-120000"), // incomplete set 3
		fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"), // complete set 4
		fmt.Sprintf(aofFilePattern, replID, "20240104-120000"), // complete set 4
	}

	actualRemainingKeys := make([]string, len(files))
	for i, file := range files {
		actualRemainingKeys[i] = file.Key
	}

	assert.ElementsMatch(t, expectedRemainingKeys, actualRemainingKeys)
}

// TestS3RetentionManagerMixedFileTypesLargeScale tests retention with many mixed files
func TestS3RetentionManagerMixedFileTypesLargeScale(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-large-scale-mixed-replication-id"

	// Generate 20 backup sets (40 files total) with timestamps over 20 days
	testFiles := make([]struct {
		key     string
		content string
	}, 0, 40)

	for i := 1; i <= 20; i++ {
		timestamp := fmt.Sprintf("202401%02d-120000", i)
		rdbContent := fmt.Sprintf("rdb content %d", i)
		aofContent := fmt.Sprintf("aof content %d", i)

		testFiles = append(testFiles, struct {
			key     string
			content string
		}{
			key:     fmt.Sprintf(rdbFilePattern, replID, timestamp),
			content: rdbContent,
		})
		testFiles = append(testFiles, struct {
			key     string
			content string
		}{
			key:     fmt.Sprintf(aofFilePattern, replID, timestamp),
			content: aofContent,
		})
	}

	// Upload all test files
	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(5 * time.Millisecond) // Small delay to ensure distinct timestamps
	}

	// Test retention with keep 5 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 5)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 10 files remaining (5 backup sets * 2 files each)
	assert.Len(t, files, 10)

	// Verify that only the 5 newest backup sets remain
	expectedRemainingTimestamps := []string{
		"20240116-120000", "20240117-120000", "20240118-120000", "20240119-120000", "20240120-120000",
	}

	actualRemainingKeys := make([]string, len(files))
	for i, file := range files {
		actualRemainingKeys[i] = file.Key
	}

	for _, timestamp := range expectedRemainingTimestamps {
		expectedRDBKey := fmt.Sprintf(rdbFilePattern, replID, timestamp)
		expectedAOFKey := fmt.Sprintf(aofFilePattern, replID, timestamp)
		assert.Contains(t, actualRemainingKeys, expectedRDBKey)
		assert.Contains(t, actualRemainingKeys, expectedAOFKey)
	}

	// Verify that older backup sets are deleted
	deletedTimestamps := []string{
		"20240101-120000", "20240102-120000", "20240103-120000", "20240115-120000",
	}

	for _, timestamp := range deletedTimestamps {
		deletedRDBKey := fmt.Sprintf(rdbFilePattern, replID, timestamp)
		deletedAOFKey := fmt.Sprintf(aofFilePattern, replID, timestamp)
		assert.NotContains(t, actualRemainingKeys, deletedRDBKey)
		assert.NotContains(t, actualRemainingKeys, deletedAOFKey)
	}
}

// TestS3RetentionManagerMixedFileTypesOnlyRDB tests retention with only RDB files
func TestS3RetentionManagerMixedFileTypesOnlyRDB(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-only-rdb-replication-id"

	// Upload test backup files with only RDB files
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf(rdbFilePattern, replID, "20240101-120000"), testRDBContent1},
		{fmt.Sprintf(rdbFilePattern, replID, "20240102-120000"), testRDBContent2},
		{fmt.Sprintf(rdbFilePattern, replID, "20240103-120000"), testRDBContent3},
		{fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"), testRDBContent4},
		{fmt.Sprintf(rdbFilePattern, replID, "20240105-120000"), testRDBContent5},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 3 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 3)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 3 files remaining (3 backup sets * 1 file each)
	assert.Len(t, files, 3)

	// Verify the correct files remain (the 3 newest backup sets)
	expectedRemainingKeys := []string{
		fmt.Sprintf(rdbFilePattern, replID, "20240103-120000"),
		fmt.Sprintf(rdbFilePattern, replID, "20240104-120000"),
		fmt.Sprintf(rdbFilePattern, replID, "20240105-120000"),
	}

	actualRemainingKeys := make([]string, len(files))
	for i, file := range files {
		actualRemainingKeys[i] = file.Key
	}

	assert.ElementsMatch(t, expectedRemainingKeys, actualRemainingKeys)
}

// TestS3RetentionManagerMixedFileTypesOnlyAOF tests retention with only AOF files
func TestS3RetentionManagerMixedFileTypesOnlyAOF(t *testing.T) {
	ctx, s3Client, cleanup := setupMinIOForRetention(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	retentionManager := backup.NewS3RetentionManager(s3Client, retentionTestBucket, logger)

	replID := "test-only-aof-replication-id"

	// Upload test backup files with only AOF files
	testFiles := []struct {
		key     string
		content string
	}{
		{fmt.Sprintf(aofFilePattern, replID, "20240101-120000"), testAOFContent1},
		{fmt.Sprintf(aofFilePattern, replID, "20240102-120000"), testAOFContent2},
		{fmt.Sprintf(aofFilePattern, replID, "20240103-120000"), testAOFContent3},
		{fmt.Sprintf(aofFilePattern, replID, "20240104-120000"), "aof content 4"},
		{fmt.Sprintf(aofFilePattern, replID, "20240105-120000"), "aof content 5"},
	}

	for _, tf := range testFiles {
		uploadTestBackupFile(ctx, t, s3Client, tf.key, tf.content)
		time.Sleep(10 * time.Millisecond)
	}

	// Test retention with keep 2 backup sets
	err := retentionManager.ManageRetention(ctx, replID, 2)
	require.NoError(t, err)

	// List remaining files
	files, err := retentionManager.ListBackupFiles(ctx, replID)
	require.NoError(t, err)

	// Should have 2 files remaining (2 backup sets * 1 file each)
	assert.Len(t, files, 2)

	// Verify the correct files remain (the 2 newest backup sets)
	expectedRemainingKeys := []string{
		fmt.Sprintf(aofFilePattern, replID, "20240104-120000"),
		fmt.Sprintf(aofFilePattern, replID, "20240105-120000"),
	}

	actualRemainingKeys := make([]string, len(files))
	for i, file := range files {
		actualRemainingKeys[i] = file.Key
	}

	assert.ElementsMatch(t, expectedRemainingKeys, actualRemainingKeys)
}
