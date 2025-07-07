package initbackup

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceInitialization(t *testing.T) {
	service := NewService()

	// Service should initialize successfully
	assert.NotNil(t, service)
	assert.NotNil(t, service.logger)
}

func TestServiceRunWithoutS3Config(t *testing.T) {
	// Clear any existing S3 environment variables
	os.Unsetenv("S3_BUCKET")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	service := NewService()
	err := service.Run()

	// Should fail without S3 configuration
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3_BUCKET environment variable is required")
}

func TestServiceRunWithS3Config(t *testing.T) {
	// Set up mock S3 configuration
	t.Setenv("S3_BUCKET", "test-bucket")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

	service := NewService()

	// Note: This will fail because we don't have a real S3 bucket
	// but we're testing that the service initializes correctly
	err := service.Run()

	// We expect some error (likely connection error), but not the config error
	if err != nil {
		assert.NotContains(t, err.Error(), "S3_BUCKET environment variable is required")
	}
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
