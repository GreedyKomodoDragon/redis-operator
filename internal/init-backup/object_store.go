package initbackup

import (
	"context"
	"io"
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

	// DownloadFileToWriter streams a file from the object store to a writer (more memory efficient)
	DownloadFileToWriter(ctx context.Context, key string, writer io.Writer) error

	// GetBucketName returns the bucket name for logging purposes
	GetBucketName() string

	// Close cleans up any resources
	Close() error
}
