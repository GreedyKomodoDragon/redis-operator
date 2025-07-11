package initbackup

import (
	"context"
	"io"
	"strings"
	"time"
)

// MockObjectStore is a mock implementation of ObjectStore for testing
type MockObjectStore struct {
	files        []ObjectStoreFile
	bucketName   string
	shouldError  bool
	errorMessage string
}

// NewMockObjectStore creates a new mock object store
func NewMockObjectStore(bucketName string) *MockObjectStore {
	return &MockObjectStore{
		bucketName: bucketName,
		files:      []ObjectStoreFile{},
	}
}

// AddFile adds a file to the mock store
func (m *MockObjectStore) AddFile(key string, lastModified time.Time, size int64) {
	m.files = append(m.files, ObjectStoreFile{
		Key:          key,
		LastModified: lastModified,
		Size:         size,
	})
}

// SetError configures the mock to return an error
func (m *MockObjectStore) SetError(shouldError bool, message string) {
	m.shouldError = shouldError
	m.errorMessage = message
}

// ListFiles implements ObjectStore.ListFiles
func (m *MockObjectStore) ListFiles(ctx context.Context, prefix string) ([]ObjectStoreFile, error) {
	if m.shouldError {
		return nil, &mockError{message: m.errorMessage}
	}

	var matchingFiles []ObjectStoreFile
	for _, file := range m.files {
		if len(prefix) == 0 || strings.HasPrefix(file.Key, prefix) {
			matchingFiles = append(matchingFiles, file)
		}
	}
	return matchingFiles, nil
}

// DownloadFileToWriter implements ObjectStore.DownloadFileToWriter
func (m *MockObjectStore) DownloadFileToWriter(ctx context.Context, key string, writer io.Writer) error {
	if m.shouldError {
		return &mockError{message: m.errorMessage}
	}

	_, err := writer.Write([]byte("mock file content"))
	return err
}

// GetBucketName implements ObjectStore.GetBucketName
func (m *MockObjectStore) GetBucketName() string {
	return m.bucketName
}

// Close implements ObjectStore.Close
func (m *MockObjectStore) Close() error {
	return nil
}

// mockError is a simple error implementation for testing
type mockError struct {
	message string
}

func (e *mockError) Error() string {
	return e.message
}
