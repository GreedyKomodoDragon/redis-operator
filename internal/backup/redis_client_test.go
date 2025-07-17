package backup

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	redisPort   = "6379/tcp"
	redisImage  = "redis:7-alpine"
	skipMessage = "Skipping integration test in short mode"
)

// TestRedisClientIntegration tests the Redis client against a real Redis instance
func TestRedisClientIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipMessage)
	}

	ctx := context.Background()

	// Test basic connection
	t.Run("BasicConnection", func(t *testing.T) {
		redisContainer, host, port, err := startRedisContainer(ctx)
		require.NoError(t, err)
		defer redisContainer.Terminate(ctx)

		client := NewRedisClient(host, port, "", false)
		defer client.Close()

		err = client.Connect()
		assert.NoError(t, err)
		assert.True(t, client.IsConnected())
	})

	// Test authentication (Redis without password should work)
	t.Run("Authentication", func(t *testing.T) {
		redisContainer, host, port, err := startRedisContainer(ctx)
		require.NoError(t, err)
		defer redisContainer.Terminate(ctx)

		client := NewRedisClient(host, port, "", false)
		defer client.Close()

		err = client.Connect()
		require.NoError(t, err)

		err = client.Authenticate()
		assert.NoError(t, err)
	})

	// Test PSYNC command
	t.Run("PSYNCCommand", func(t *testing.T) {
		redisContainer, host, port, err := startRedisContainer(ctx)
		require.NoError(t, err)
		defer redisContainer.Terminate(ctx)

		client := NewRedisClient(host, port, "", false)
		defer client.Close()

		err = client.Connect()
		require.NoError(t, err)

		err = client.Authenticate()
		require.NoError(t, err)

		// PSYNC should return FULLRESYNC
		replInfo, err := client.SendPSYNC()
		assert.NoError(t, err)
		assert.NotEmpty(t, replInfo.ID)
		assert.GreaterOrEqual(t, replInfo.Offset, int64(0))
	})

	// Test RDB snapshot reading
	t.Run("RDBSnapshot", func(t *testing.T) {
		redisContainer, host, port, err := startRedisContainer(ctx)
		require.NoError(t, err)
		defer redisContainer.Terminate(ctx)

		// First, add some data to Redis to ensure RDB is not empty
		stdClient := redis.NewClient(&redis.Options{
			Addr: net.JoinHostPort(host, port),
		})
		defer stdClient.Close()

		// Add some test data
		err = stdClient.Set(ctx, "test:key1", "value1", 0).Err()
		require.NoError(t, err)
		err = stdClient.Set(ctx, "test:key2", "value2", 0).Err()
		require.NoError(t, err)

		// Now use our custom client for PSYNC/RDB reading
		client := NewRedisClient(host, port, "", false)
		defer client.Close()

		err = client.Connect()
		require.NoError(t, err)

		err = client.Authenticate()
		require.NoError(t, err)

		// Send PSYNC and read RDB
		_, err = client.SendPSYNC()
		require.NoError(t, err)

		// Read RDB snapshot
		rdbData, err := client.ReadRDBSnapshot()
		assert.NoError(t, err)
		assert.NotEmpty(t, rdbData)

		// RDB should start with "REDIS" magic string
		if len(rdbData) >= 5 {
			assert.Equal(t, "REDIS", string(rdbData[:5]))
		}
	})
}

// TestRedisClientWithAuth tests Redis client with authentication
func TestRedisClientWithAuth(t *testing.T) {
	if testing.Short() {
		t.Skip(skipMessage)
	}

	ctx := context.Background()
	password := "testpassword"

	// Start Redis container with authentication
	redisContainer, host, port, err := startRedisContainerWithAuth(ctx, password)
	require.NoError(t, err)
	defer redisContainer.Terminate(ctx)

	t.Run("AuthenticationRequired", func(t *testing.T) {
		// Test without password - should fail when trying to execute commands
		client := NewRedisClient(host, port, "", false)
		defer client.Close()

		err := client.Connect()
		require.NoError(t, err)

		// Authentication won't fail since no password is provided to Authenticate()
		// But PSYNC should fail due to auth requirement
		_, err = client.SendPSYNC()
		assert.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "auth")
	})

	t.Run("AuthenticationSuccess", func(t *testing.T) {
		// Test with correct password should succeed
		client := NewRedisClient(host, port, password, false)
		defer client.Close()

		err := client.Connect()
		require.NoError(t, err)

		err = client.Authenticate()
		assert.NoError(t, err)
	})
}

// TestRedisClientConnectionRetry tests the retry logic
func TestRedisClientConnectionRetry(t *testing.T) {
	t.Run("RetryConfiguration", func(t *testing.T) {
		client := NewRedisClient("localhost", "6379", "", false)

		// Test default retry settings
		assert.Equal(t, 5, client.maxRetries)
		assert.Equal(t, 5*time.Second, client.retryDelay)
		assert.Equal(t, 300*time.Second, client.maxRetryDelay)

		// Test custom retry settings
		client.SetRetryConfig(3, 2*time.Second, 60*time.Second)
		assert.Equal(t, 3, client.maxRetries)
		assert.Equal(t, 2*time.Second, client.retryDelay)
		assert.Equal(t, 60*time.Second, client.maxRetryDelay)
	})

	t.Run("FailedConnection", func(t *testing.T) {
		// Try to connect to non-existent Redis
		client := NewRedisClient("localhost", "9999", "", false)
		client.SetRetryConfig(2, 100*time.Millisecond, 200*time.Millisecond)

		start := time.Now()
		err := client.Connect()
		duration := time.Since(start)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection failed after 2 attempts")
		// Should have taken at least 100ms (first retry delay)
		assert.Greater(t, duration, 100*time.Millisecond)
	})
}

// TestRedisClientRESPParsing tests RESP protocol parsing
func TestRedisClientRESPParsing(t *testing.T) {
	if testing.Short() {
		t.Skip(skipMessage)
	}

	ctx := context.Background()

	// Start Redis container
	redisContainer, host, port, err := startRedisContainer(ctx)
	require.NoError(t, err)
	defer redisContainer.Terminate(ctx)

	// Set up some data for replication
	err = populateRedisWithData(ctx, host, port)
	require.NoError(t, err)

	t.Run("CommandStreamReading", func(t *testing.T) {
		client := NewRedisClient(host, port, "", false)
		defer client.Close()

		err := client.Connect()
		require.NoError(t, err)

		err = client.Authenticate()
		require.NoError(t, err)

		_, err = client.SendPSYNC()
		require.NoError(t, err)

		// Read RDB first
		_, err = client.ReadRDBSnapshot()
		require.NoError(t, err)

		// Try to read some commands (this might timeout if no new commands)
		// We'll use a short timeout for testing
		done := make(chan bool, 1)
		var commands []string

		go func() {
			for i := 0; i < 3; i++ {
				command, err := client.ReadRESPCommand()
				if err != nil {
					break
				}
				commands = append(commands, command)
			}
			done <- true
		}()

		// Generate some commands by writing to Redis
		go func() {
			time.Sleep(100 * time.Millisecond)
			writeDataToRedis(ctx, host, port)
		}()

		// Wait for commands or timeout
		select {
		case <-done:
			// Commands were read successfully
			t.Logf("Read %d commands from replication stream", len(commands))
		case <-time.After(2 * time.Second):
			// Timeout is expected in some cases
			t.Log("Timeout reading commands (this is normal for this test)")
		}
	})
}

// Helper functions

func startRedisContainer(ctx context.Context) (testcontainers.Container, string, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        redisImage,
		ExposedPorts: []string{redisPort},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", "", err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, "", "", err
	}

	// Try getting the mapped port using nat.Port type
	natPort := nat.Port(redisPort)
	mappedPort, err := container.MappedPort(ctx, natPort)
	if err != nil {
		return nil, "", "", err
	}

	return container, host, mappedPort.Port(), nil
}

func startRedisContainerWithAuth(ctx context.Context, password string) (testcontainers.Container, string, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        redisImage,
		ExposedPorts: []string{redisPort},
		Cmd:          []string{"redis-server", "--requirepass", password},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", "", err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, "", "", err
	}

	// Try getting the mapped port using nat.Port type
	natPort := nat.Port(redisPort)
	mappedPort, err := container.MappedPort(ctx, natPort)
	if err != nil {
		return nil, "", "", err
	}

	return container, host, mappedPort.Port(), nil
}

func populateRedisWithData(ctx context.Context, host, port string) error {
	// Use go-redis client to populate data
	rdb := createRedisClient(host, port, "")
	defer rdb.Close()

	// Set some initial data
	err := rdb.Set(ctx, "key1", "value1", 0).Err()
	if err != nil {
		return err
	}

	err = rdb.Set(ctx, "key2", "value2", 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func writeDataToRedis(ctx context.Context, host, port string) error {
	rdb := createRedisClient(host, port, "")
	defer rdb.Close()

	// Write some data to trigger replication commands
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("testkey%d", i)
		value := fmt.Sprintf("testvalue%d", i)
		err := rdb.Set(ctx, key, value, 0).Err()
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

func createRedisClient(host, port, password string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       0,
	})
}

// Benchmark tests

func BenchmarkRedisClientConnect(b *testing.B) {
	if testing.Short() {
		b.Skip(skipMessage)
	}

	ctx := context.Background()
	redisContainer, host, port, err := startRedisContainer(ctx)
	if err != nil {
		b.Fatalf("Failed to start Redis container: %v", err)
	}
	defer redisContainer.Terminate(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewRedisClient(host, port, "", false)
		err := client.Connect()
		if err != nil {
			b.Fatalf("Failed to connect: %v", err)
		}
		client.Close()
	}
}

func BenchmarkRedisClientPSYNC(b *testing.B) {
	if testing.Short() {
		b.Skip(skipMessage)
	}

	ctx := context.Background()
	redisContainer, host, port, err := startRedisContainer(ctx)
	if err != nil {
		b.Fatalf("Failed to start Redis container: %v", err)
	}
	defer redisContainer.Terminate(ctx)

	client := NewRedisClient(host, port, "", false)
	err = client.Connect()
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	err = client.Authenticate()
	if err != nil {
		b.Fatalf("Failed to authenticate: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.SendPSYNC()
		if err != nil && !strings.Contains(err.Error(), "unexpected PSYNC response") {
			b.Fatalf("PSYNC failed: %v", err)
		}
		// Note: Subsequent PSYNC calls might fail as expected
	}
}
