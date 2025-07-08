package initbackup

import (
	"context"
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
		if len(prefix) == 0 || hasPrefix(file.Key, prefix) {
			matchingFiles = append(matchingFiles, file)
		}
	}
	return matchingFiles, nil
}

// DownloadFile implements ObjectStore.DownloadFile
func (m *MockObjectStore) DownloadFile(ctx context.Context, key string) ([]byte, error) {
	if m.shouldError {
		return nil, &mockError{message: m.errorMessage}
	}
	return []byte("mock file content"), nil
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

// hasPrefix checks if a string has a given prefix (helper function)
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
