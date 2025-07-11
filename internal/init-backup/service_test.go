package initbackup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTimestamp = "20241225-120000"
	testReplID    = "abc123"
	testAOFKey    = "redis-backups/abc123/appendonly-20241225-120000.aof"
	testBucket    = "test-bucket"
)

func TestServiceInitialization(t *testing.T) {
	mockStore := NewMockObjectStore(testBucket)
	service := NewService(mockStore)

	// Service should initialize successfully
	assert.NotNil(t, service)
	assert.NotNil(t, service.logger)
	assert.NotNil(t, service.objectStore)
}

func TestServiceRunWithoutS3Config(t *testing.T) {
	// Test with mock store that returns an error
	mockStore := NewMockObjectStore(testBucket)
	mockStore.SetError(true, "connection failed")

	service := NewService(mockStore)
	err := service.Run("/tmp/test-data")

	// Should fail due to mock error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection failed")
}

func TestServiceRunWithS3Config(t *testing.T) {
	// Test with mock store that works
	mockStore := NewMockObjectStore(testBucket)

	service := NewService(mockStore)
	err := service.Run("/tmp/test-data")

	// Should succeed (no files found, but no error)
	assert.NoError(t, err)
}

func TestGetEnvOrDefault(t *testing.T) {
	// Test with existing environment variable
	t.Setenv("TEST_VAR", "test_value")
	result := getEnvOrDefault("TEST_VAR", "default_value")
	assert.Equal(t, "test_value", result)

	// Test with non-existing environment variable
	result = getEnvOrDefault("NON_EXISTING_VAR", "default_value")
	assert.Equal(t, "default_value", result)
}

func TestParseRDBFile(t *testing.T) {
	mockStore := NewMockObjectStore(testBucket)
	service := NewService(mockStore)
	timestamp := time.Now()

	tests := []struct {
		name        string
		key         string
		shouldError bool
		expectedID  string
		expectedTS  string
	}{
		{
			name:        "valid RDB file",
			key:         "redis-backups/abc123/dump-" + testTimestamp + ".rdb",
			shouldError: false,
			expectedID:  testReplID,
			expectedTS:  testTimestamp,
		},
		{
			name:        "invalid path format",
			key:         "invalid/path.rdb",
			shouldError: true,
		},
		{
			name:        "invalid filename format",
			key:         "redis-backups/abc123/invalid.rdb",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdbFile, err := service.parseRDBFile(tt.key, timestamp, 1024)

			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, rdbFile)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, rdbFile)
				assert.Equal(t, tt.expectedID, rdbFile.ReplicationID)
				assert.Equal(t, tt.expectedTS, rdbFile.Timestamp)
				assert.Equal(t, tt.key, rdbFile.Key)
			}
		})
	}
}

func TestParseAOFFile(t *testing.T) {
	mockStore := NewMockObjectStore(testBucket)
	service := NewService(mockStore)
	timestamp := time.Now()

	tests := []struct {
		name        string
		key         string
		shouldError bool
		expectedID  string
		expectedTS  string
	}{
		{
			name:        "valid AOF file",
			key:         testAOFKey,
			shouldError: false,
			expectedID:  testReplID,
			expectedTS:  testTimestamp,
		},
		{
			name:        "invalid path format",
			key:         "invalid/path.aof",
			shouldError: true,
		},
		{
			name:        "invalid filename format",
			key:         "redis-backups/abc123/invalid.aof",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aofFile, err := service.parseAOFFile(tt.key, timestamp, 2048)

			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, aofFile)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, aofFile)
				assert.Equal(t, tt.expectedID, aofFile.ReplicationID)
				assert.Equal(t, tt.expectedTS, aofFile.Timestamp)
				assert.Equal(t, tt.key, aofFile.Key)
			}
		})
	}
}

func TestFindMatchingAOFFiles(t *testing.T) {
	mockStore := NewMockObjectStore(testBucket)
	service := NewService(mockStore)

	rdbFile := &RDBFile{
		Key:           "redis-backups/abc123/dump-20241225-120000.rdb",
		ReplicationID: "abc123",
		Timestamp:     "20241225-120000",
	}

	aofFiles := []AOFFile{
		{
			Key:           "redis-backups/abc123/appendonly-20241225-120000.aof",
			ReplicationID: "abc123",
			Timestamp:     "20241225-120000",
		},
		{
			Key:           "redis-backups/def456/appendonly-20241225-120000.aof",
			ReplicationID: "def456", // Different replication ID
			Timestamp:     "20241225-120000",
		},
		{
			Key:           "redis-backups/abc123/appendonly-20241225-130000.aof",
			ReplicationID: "abc123",
			Timestamp:     "20241225-130000", // Different timestamp
		},
	}

	matchingAOFs := service.findMatchingAOFFiles(rdbFile, aofFiles)

	// Should only match the first AOF file (same replication ID and timestamp)
	assert.Len(t, matchingAOFs, 1)
	assert.Equal(t, "redis-backups/abc123/appendonly-20241225-120000.aof", matchingAOFs[0].Key)
}
