package backup

import (
	"context"
	"io"
	"time"
)

// BackupFile represents a backup file in S3
type BackupFile struct {
	Key          string
	LastModified time.Time
	Size         int64
}

// Uploader interface for handling backup uploads
type Uploader interface {
	// UploadStream uploads a stream to the backup storage and returns location
	UploadStream(ctx context.Context, reader io.Reader, key string, metadata map[string]string) (string, error)

	// UploadStreamWithProgress uploads a stream with progress reporting and returns location
	UploadStreamWithProgress(ctx context.Context, reader io.Reader, key string, metadata map[string]string, progressCallback func(bytes int64)) (string, error)
}

// RetentionManager interface for handling backup retention
type RetentionManager interface {
	// ManageRetention applies retention policies to backup files
	ManageRetention(ctx context.Context, replicationID string, retentionCount int) error

	// GetFilesToDelete returns files that should be deleted based on retention policy
	GetFilesToDelete(files []BackupFile, retentionCount int) ([]BackupFile, error)
}
