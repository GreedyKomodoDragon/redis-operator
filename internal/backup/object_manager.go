package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

// BackupObjectManager coordinates uploads and retention management
type BackupObjectManager struct {
	uploader         Uploader
	retentionManager RetentionManager
	logger           *slog.Logger
}

// NewBackupObjectManager creates a new backup object manager
func NewBackupObjectManager(uploader Uploader, retentionManager RetentionManager, logger *slog.Logger) *BackupObjectManager {
	return &BackupObjectManager{
		uploader:         uploader,
		retentionManager: retentionManager,
		logger:           logger,
	}
}

// UploadWithProgressAndRetention uploads a backup with progress reporting and applies retention policy
func (bom *BackupObjectManager) UploadWithProgressAndRetention(ctx context.Context, reader io.Reader, key string, metadata map[string]string, replicationID string, retentionCount int, progressCallback func(bytes int64)) error {
	// Upload the backup with progress reporting
	_, err := bom.uploader.UploadStreamWithProgress(ctx, reader, key, metadata, progressCallback)
	if err != nil {
		return fmt.Errorf("failed to upload backup: %w", err)
	}

	// Apply retention policy if enabled
	if retentionCount > 0 {
		if err := bom.retentionManager.ManageRetention(ctx, replicationID, retentionCount); err != nil {
			bom.logger.Warn("Failed to apply retention policy", "error", err)
			// Don't fail the upload if retention fails
		}
	}

	return nil
}
