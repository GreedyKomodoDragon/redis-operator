package initbackup

import (
	"context"
	"time"
)

// ObjectStoreFile represents a file stored in an object store
type ObjectStoreFile struct {
	Key          string
	LastModified time.Time
	Size         int64
}

// ObjectStore defines the interface for object storage backends
type ObjectStore interface {
	// ListFiles lists all files in the object store with the given prefix
	ListFiles(ctx context.Context, prefix string) ([]ObjectStoreFile, error)

	// DownloadFile downloads a file from the object store
	// This method is not implemented yet but will be needed for restore functionality
	DownloadFile(ctx context.Context, key string) ([]byte, error)

	// GetBucketName returns the bucket name for logging purposes
	GetBucketName() string

	// Close cleans up any resources
	Close() error
}
