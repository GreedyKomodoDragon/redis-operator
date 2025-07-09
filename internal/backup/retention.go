package backup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3RetentionManager handles retention management for S3 backups
type S3RetentionManager struct {
	logger *slog.Logger
	client *s3.Client
	bucket string
}

// NewS3RetentionManager creates a new S3 retention manager
func NewS3RetentionManager(client *s3.Client, bucket string, logger *slog.Logger) *S3RetentionManager {
	return &S3RetentionManager{
		logger: logger,
		client: client,
		bucket: bucket,
	}
}

// ManageRetention applies retention policies to backup files
func (rm *S3RetentionManager) ManageRetention(ctx context.Context, replicationID string, retentionCount int) error {
	if retentionCount <= 0 {
		rm.logger.Info("Retention disabled (retention count <= 0)")
		return nil
	}

	rm.logger.Info("Managing backup retention", "retention_count", retentionCount, "replication_id", replicationID)

	// List all backup files for this replication ID
	backupFiles, err := rm.ListBackupFiles(ctx, replicationID)
	if err != nil {
		return fmt.Errorf("failed to list backup files: %w", err)
	}

	// Get files to delete based on retention policy
	filesToDelete, err := rm.GetFilesToDelete(backupFiles, retentionCount)
	if err != nil {
		return fmt.Errorf("failed to determine files to delete: %w", err)
	}

	if len(filesToDelete) == 0 {
		rm.logger.Info("No files to delete after retention analysis")
		return nil
	}

	// Extract keys for deletion
	keysToDelete := make([]string, len(filesToDelete))
	for i, file := range filesToDelete {
		keysToDelete[i] = file.Key
	}

	rm.logger.Info("Deleting old backup files", "count", len(keysToDelete), "files", keysToDelete)

	// Delete the old files
	if err := rm.DeleteBackupFiles(ctx, keysToDelete); err != nil {
		return fmt.Errorf("failed to delete old backup files: %w", err)
	}

	rm.logger.Info("Successfully cleaned up old backups", "deleted_count", len(keysToDelete))
	return nil
}

// GetFilesToDelete returns files that should be deleted based on retention policy
func (rm *S3RetentionManager) GetFilesToDelete(files []BackupFile, retentionCount int) ([]BackupFile, error) {
	if retentionCount <= 0 || len(files) <= retentionCount {
		return nil, nil
	}

	// Group files by timestamp
	backupSets := rm.groupBackupsByTimestamp(files)

	// Sort backup sets by timestamp (newest first)
	timestamps := make([]string, 0, len(backupSets))
	for timestamp := range backupSets {
		timestamps = append(timestamps, timestamp)
	}

	// Sort timestamps in descending order (newest first)
	for i := 0; i < len(timestamps)-1; i++ {
		for j := i + 1; j < len(timestamps); j++ {
			if timestamps[i] < timestamps[j] {
				timestamps[i], timestamps[j] = timestamps[j], timestamps[i]
			}
		}
	}

	// Determine which backup sets to delete (keep only the latest N sets)
	if len(timestamps) <= retentionCount {
		return nil, nil // Nothing to delete
	}

	var filesToDelete []BackupFile
	for i := retentionCount; i < len(timestamps); i++ {
		timestamp := timestamps[i]
		filesToDelete = append(filesToDelete, backupSets[timestamp]...)
	}

	return filesToDelete, nil
}

// groupBackupsByTimestamp groups backup files by their timestamp
func (rm *S3RetentionManager) groupBackupsByTimestamp(files []BackupFile) map[string][]BackupFile {
	groups := make(map[string][]BackupFile)

	for _, file := range files {
		// Extract timestamp from filename
		// Expected formats: redis-backups/{replID}/dump-20240101-120000.rdb
		//                  redis-backups/{replID}/appendonly-20240101-120000.aof

		var timestamp string
		if len(file.Key) > 0 {
			// Split by '/' and get the filename part
			parts := strings.Split(file.Key, "/")

			// Validate path structure: redis-backups/{replID}/filename
			if len(parts) >= 3 && parts[0] == "redis-backups" {
				filename := parts[len(parts)-1] // Get the last part (filename)

				// Extract timestamp from filename
				if strings.HasPrefix(filename, "dump-") && strings.HasSuffix(filename, ".rdb") {
					// dump-20240101-120000.rdb -> 20240101-120000
					extractedTimestamp := strings.TrimPrefix(strings.TrimSuffix(filename, ".rdb"), "dump-")
					if rm.isValidTimestamp(extractedTimestamp) {
						timestamp = extractedTimestamp
					}
				} else if strings.HasPrefix(filename, "appendonly-") && strings.HasSuffix(filename, ".aof") {
					// appendonly-20240101-120000.aof -> 20240101-120000
					extractedTimestamp := strings.TrimPrefix(strings.TrimSuffix(filename, ".aof"), "appendonly-")
					if rm.isValidTimestamp(extractedTimestamp) {
						timestamp = extractedTimestamp
					}
				}
			}
		}

		if timestamp != "" {
			groups[timestamp] = append(groups[timestamp], file)
		} else {
			// Only log if logger is initialized
			if rm.logger != nil {
				rm.logger.Warn("Could not extract timestamp from backup file", "key", file.Key)
			}
		}
	}

	return groups
}

// ListBackupFiles lists all backup files (RDB and AOF) for a specific replication ID
func (u *S3RetentionManager) ListBackupFiles(ctx context.Context, replID string) ([]BackupFile, error) {
	prefix := fmt.Sprintf("redis-backups/%s/", replID)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(u.bucket),
		Prefix: aws.String(prefix),
	}

	var backupFiles []BackupFile
	paginator := s3.NewListObjectsV2Paginator(u.client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %v", err)
		}

		for _, obj := range output.Contents {
			size := int64(0)
			if obj.Size != nil {
				size = *obj.Size
			}
			backupFiles = append(backupFiles, BackupFile{
				Key:          *obj.Key,
				LastModified: *obj.LastModified,
				Size:         size,
			})
		}
	}

	return backupFiles, nil
}

// DeleteBackupFiles deletes multiple backup files from S3
func (u *S3RetentionManager) DeleteBackupFiles(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	// For compatibility with MinIO, use individual delete operations
	// instead of bulk delete which requires Content-MD5 header
	for _, key := range keys {
		_, err := u.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(u.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return fmt.Errorf("failed to delete object %s: %v", key, err)
		}
	}

	return nil
}

// isValidTimestamp checks if a timestamp string matches the expected format: YYYYMMDD-HHMMSS
func (rm *S3RetentionManager) isValidTimestamp(timestamp string) bool {
	// Expected format: 20240101-120000
	if len(timestamp) != 15 {
		return false
	}

	// Check for dash in the right position
	if timestamp[8] != '-' {
		return false
	}

	// Check if the date and time parts are numeric
	datePart := timestamp[:8] // YYYYMMDD
	timePart := timestamp[9:] // HHMMSS

	for _, r := range datePart {
		if r < '0' || r > '9' {
			return false
		}
	}

	for _, r := range timePart {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}
