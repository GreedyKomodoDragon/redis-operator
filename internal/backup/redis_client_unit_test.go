package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNewRedisClient tests Redis client creation
func TestNewRedisClient(t *testing.T) {
	client := NewRedisClient("localhost", "6379", "password", true)

	assert.Equal(t, "localhost", client.host)
	assert.Equal(t, "6379", client.port)
	assert.Equal(t, "password", client.password)
	assert.True(t, client.tlsEnabled)
	assert.False(t, client.IsConnected())

	// Test default retry settings
	assert.Equal(t, 5, client.maxRetries)
	assert.Equal(t, 5*time.Second, client.retryDelay)
	assert.Equal(t, 300*time.Second, client.maxRetryDelay)
}

// TestRedisClientSetRetryConfig tests retry configuration
func TestRedisClientSetRetryConfig(t *testing.T) {
	client := NewRedisClient("localhost", "6379", "", false)

	client.SetRetryConfig(3, 2*time.Second, 60*time.Second)

	assert.Equal(t, 3, client.maxRetries)
	assert.Equal(t, 2*time.Second, client.retryDelay)
	assert.Equal(t, 60*time.Second, client.maxRetryDelay)
}

// TestRedisClientClose tests close functionality
func TestRedisClientClose(t *testing.T) {
	client := NewRedisClient("localhost", "6379", "", false)

	// Close should not error even when not connected
	err := client.Close()
	assert.NoError(t, err)
	assert.False(t, client.IsConnected())
}

// TestRedisClientAuthenticationWithEmptyPassword tests auth with empty password
func TestRedisClientAuthenticationWithEmptyPassword(t *testing.T) {
	client := NewRedisClient("localhost", "6379", "", false)

	// Should return no error for empty password
	err := client.Authenticate()
	assert.NoError(t, err)
}
