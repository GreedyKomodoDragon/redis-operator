package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"
)

type BackupService struct {
	s3Bucket    string
	s3Enabled   bool
	redisClient *RedisClient
	s3Uploader  *S3Uploader

	// Replication state
	replID     string
	replOffset int64
}

type ReplicationInfo struct {
	ID     string
	Offset int64
}

func Run() {
	// Initialize the backup service
	backupService := &BackupService{}

	if err := backupService.initialize(); err != nil {
		panic(fmt.Errorf("failed to initialize backup service: %v", err))
	}

	// Start the backup process
	if err := backupService.Start(context.Background()); err != nil {
		panic(fmt.Errorf("backup service failed: %v", err))
	}
}

func (s *BackupService) initialize() error {
	// Get Redis connection details from environment variables
	redisHost := getEnvOrDefault("REDIS_HOST", "localhost")
	redisPort := getEnvOrDefault("REDIS_PORT", "6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	tlsEnabled := getEnvOrDefault("REDIS_TLS_ENABLED", "false") == "true"

	// Create Redis client
	s.redisClient = NewRedisClient(redisHost, redisPort, redisPassword, tlsEnabled)

	// Configure retry settings
	maxRetries := getEnvIntOrDefault("MAX_RETRIES", 5)
	retryDelay := time.Duration(getEnvIntOrDefault("RETRY_DELAY_SECONDS", 5)) * time.Second
	maxRetryDelay := time.Duration(getEnvIntOrDefault("MAX_RETRY_DELAY_SECONDS", 300)) * time.Second
	s.redisClient.SetRetryConfig(maxRetries, retryDelay, maxRetryDelay)

	// S3 configuration - required for backup
	s.s3Bucket = os.Getenv("S3_BUCKET")
	s.s3Enabled = s.s3Bucket != ""

	if !s.s3Enabled {
		return fmt.Errorf("S3_BUCKET environment variable is required - backup service only supports S3 storage")
	}

	// Initialize S3 uploader
	s3Config := S3Config{
		Bucket:          s.s3Bucket,
		Region:          getEnvOrDefault("AWS_REGION", "us-east-1"),
		Endpoint:        os.Getenv("AWS_ENDPOINT_URL"),
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uploader, err := NewS3Uploader(ctx, s3Config)
	if err != nil {
		return fmt.Errorf("failed to create S3 uploader: %v", err)
	}
	s.s3Uploader = uploader

	fmt.Printf("Backup service initialized - Redis: %s:%s, TLS: %v, S3: %v\n",
		redisHost, redisPort, tlsEnabled, s.s3Enabled)
	return nil
}

func (s *BackupService) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Backup service shutting down...")
			return ctx.Err()
		default:
			fmt.Println("Starting backup service as Redis replica...")

			if err := s.runAsReplica(ctx); err != nil {
				fmt.Printf("Replica connection failed: %v\n", err)

				// Try graceful reconnect
				if reconnectErr := s.gracefulReconnect(); reconnectErr != nil {
					fmt.Printf("Graceful reconnect failed: %v\n", reconnectErr)
				}

				fmt.Println("Retrying replica connection in 30 seconds...")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(30 * time.Second):
					continue
				}
			}

			fmt.Println("Replica connection ended, attempting to reconnect...")
			time.Sleep(5 * time.Second)
		}
	}
}

// runAsReplica establishes a persistent connection to Redis and acts as a replica
func (s *BackupService) runAsReplica(ctx context.Context) error {
	// Step 1: Connect to Redis
	if err := s.redisClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}
	defer s.redisClient.Close()

	// Step 2: Authenticate if password is provided
	if err := s.redisClient.Authenticate(); err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	// Step 3: Send PSYNC command and handle full resync
	replInfo, err := s.redisClient.SendPSYNC()
	if err != nil {
		return fmt.Errorf("PSYNC failed: %v", err)
	}

	s.replID = replInfo.ID
	s.replOffset = replInfo.Offset
	fmt.Printf("Acting as Redis replica - ID: %s, Offset: %d\n", s.replID, s.replOffset)

	// Step 4: Stream RDB snapshot directly to S3
	if err := s.streamRDBToS3(); err != nil {
		return fmt.Errorf("failed to stream RDB to S3: %v", err)
	}

	// Step 5: Continuously stream commands as they arrive (act as persistent replica)
	if err := s.streamCommandsToS3(ctx); err != nil {
		return fmt.Errorf("failed to stream commands to S3: %v", err)
	}

	return nil
}

func (s *BackupService) streamRDBToS3() error {
	fmt.Println("Streaming RDB snapshot directly to S3...")

	// Read RDB data from Redis
	rdbData, err := s.redisClient.ReadRDBSnapshot()
	if err != nil {
		return fmt.Errorf("failed to read RDB data: %v", err)
	}

	// Create a reader from the RDB data
	reader := bytes.NewReader(rdbData)

	// Generate S3 key
	timestamp := time.Now().Format("20060102-150405")
	s3Key := fmt.Sprintf("redis-backups/%s/dump-%s.rdb", s.replID, timestamp)

	// Metadata for the backup
	metadata := map[string]string{
		"uploaded-by":        "redis-operator",
		"timestamp":          fmt.Sprintf("%d", time.Now().Unix()),
		"replication-id":     s.replID,
		"replication-offset": fmt.Sprintf("%d", s.replOffset),
		"backup-type":        "rdb",
	}

	// Upload stream to S3 with progress tracking
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	location, err := s.s3Uploader.UploadStreamWithProgress(ctx, reader, s3Key, metadata, func(bytes int64) {
		if bytes%(1024*1024) == 0 { // Log every MB
			fmt.Printf("Uploaded RDB: %d bytes\n", bytes)
		}
	})

	if err != nil {
		return fmt.Errorf("failed to upload RDB to S3: %v", err)
	}

	fmt.Printf("RDB snapshot successfully uploaded to S3: %s (%d bytes)\n", location, len(rdbData))
	return nil
}

func (s *BackupService) streamCommandsToS3(ctx context.Context) error {
	fmt.Println("Starting persistent command stream backup to S3...")

	// Create a pipe for streaming commands
	reader, writer := io.Pipe()

	// Generate S3 key
	timestamp := time.Now().Format("20060102-150405")
	s3Key := fmt.Sprintf("redis-backups/%s/appendonly-%s.aof", s.replID, timestamp)

	// Metadata for the backup
	metadata := map[string]string{
		"uploaded-by":        "redis-operator",
		"timestamp":          fmt.Sprintf("%d", time.Now().Unix()),
		"replication-id":     s.replID,
		"replication-offset": fmt.Sprintf("%d", s.replOffset),
		"backup-type":        "aof",
	}

	// Start S3 upload in a goroutine
	uploadComplete := make(chan error, 1)
	go s.uploadStreamToS3(reader, s3Key, metadata, uploadComplete)

	// Stream commands to the pipe - act as a persistent replica
	defer writer.Close()

	return s.streamReplicationData(ctx, writer, uploadComplete)
}

// streamReplicationData handles the actual streaming of replication data
func (s *BackupService) streamReplicationData(ctx context.Context, writer *io.PipeWriter, uploadComplete <-chan error) error {
	commandCount := 0
	// Use longer timeout for replication connections - Redis replicas can be idle for long periods
	commandTimeout := 5 * time.Minute     // Increased from 30 seconds
	keepaliveInterval := 30 * time.Second // More frequent keepalive
	lastKeepalive := time.Now()

	fmt.Println("Acting as Redis replica - continuously waiting for commands...")
	fmt.Println("This will run until the Redis master disconnects or context is cancelled")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Context cancelled, stopping replica...")
			writer.CloseWithError(ctx.Err())
			return ctx.Err()
		case err := <-uploadComplete:
			return s.handleUploadComplete(err)
		default:
			if s.shouldSendKeepalive(lastKeepalive, keepaliveInterval) {
				lastKeepalive = time.Now()
			}

			command, err := s.readReplicationCommand(commandTimeout)
			if err != nil {
				return s.handleReplicationError(err, writer)
			}

			if err := s.writeCommandToStream(writer, command); err != nil {
				return err
			}

			commandCount++
			if commandCount%10 == 0 {
				fmt.Printf("Replicated %d commands...\n", commandCount)
			}
		}
	}
}

// handleUploadComplete handles the completion of S3 upload
func (s *BackupService) handleUploadComplete(err error) error {
	if err != nil {
		fmt.Printf("Upload failed: %v\n", err)
		return err
	}
	fmt.Println("Upload completed successfully")
	return nil
}

// shouldSendKeepalive checks if keepalive should be sent
func (s *BackupService) shouldSendKeepalive(lastKeepalive time.Time, interval time.Duration) bool {
	if time.Since(lastKeepalive) > interval {
		fmt.Println("Sending replication keepalive (REPLCONF ACK)...")

		// Send REPLCONF ACK to maintain replication connection
		// This is the proper way to keepalive a replication connection
		if err := s.sendReplconfAck(); err != nil {
			fmt.Printf("Warning: Failed to send REPLCONF ACK: %v\n", err)
		} else {
			fmt.Println("REPLCONF ACK sent successfully")
		}

		return true
	}
	return false
}

// sendReplconfAck sends a REPLCONF ACK command to maintain replication connection
func (s *BackupService) sendReplconfAck() error {
	// Send REPLCONF ACK with current offset
	replconfCmd := fmt.Sprintf("REPLCONF ACK %d\r\n", s.replOffset)
	return s.redisClient.SendCommand(replconfCmd)
}

// readReplicationCommand reads a command from the replication stream
func (s *BackupService) readReplicationCommand(timeout time.Duration) (string, error) {
	command, err := s.redisClient.ReadRESPCommandWithTimeout(timeout)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// Timeout is normal for replicas with low traffic
			return "", nil
		}
		return "", err
	}
	return command, nil
}

// handleReplicationError handles errors during replication
func (s *BackupService) handleReplicationError(err error, writer *io.PipeWriter) error {
	if err == nil {
		return nil // No error, continue
	}

	if err == io.EOF {
		fmt.Println("Master closed replication connection (EOF)")
		fmt.Println("This can happen due to:")
		fmt.Println("  - Redis master timeout for idle connections")
		fmt.Println("  - Network timeout/firewall rules")
		fmt.Println("  - Redis restart or configuration changes")
		fmt.Println("  - Maximum replica connections reached")
		return nil
	}

	// Check for specific network errors
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			fmt.Printf("Network timeout during replication: %v\n", err)
		} else {
			fmt.Printf("Network error during replication: %v\n", err)
		}
	} else {
		fmt.Printf("Replication error: %v\n", err)
	}

	writer.CloseWithError(err)
	return err
}

// writeCommandToStream writes a command to the backup stream
func (s *BackupService) writeCommandToStream(writer *io.PipeWriter, command string) error {
	if command == "" {
		return nil // Skip empty commands (timeouts)
	}

	if _, err := writer.Write([]byte(command)); err != nil {
		fmt.Printf("Failed to write command to backup stream: %v\n", err)
		return err
	}
	return nil
}

// gracefulReconnect handles reconnection and restart of backup cycle
func (s *BackupService) gracefulReconnect() error {
	fmt.Println("Attempting graceful reconnect...")

	// Close existing connection
	s.redisClient.Close()

	// Wait before reconnecting
	time.Sleep(5 * time.Second)

	// Try to reconnect
	if err := s.redisClient.Connect(); err != nil {
		return fmt.Errorf("failed to reconnect to Redis: %v", err)
	}

	// Test the connection
	if err := s.redisClient.Authenticate(); err != nil {
		s.redisClient.Close()
		return fmt.Errorf("authentication failed after reconnect: %v", err)
	}

	fmt.Println("Successfully reconnected to Redis")
	s.redisClient.Close() // Close for now, will reconnect in next cycle
	return nil
}

// Helper functions for environment variables
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func (s *BackupService) uploadStreamToS3(reader io.Reader, s3Key string, metadata map[string]string, uploadComplete chan<- error) {
	defer close(uploadComplete)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	location, err := s.s3Uploader.UploadStreamWithProgress(ctx, reader, s3Key, metadata, func(bytes int64) {
		if bytes%(1024*1024) == 0 { // Log every MB
			fmt.Printf("Uploaded AOF: %d bytes\n", bytes)
		}
	})

	if err != nil {
		uploadComplete <- fmt.Errorf("failed to upload AOF to S3: %v", err)
		return
	}

	fmt.Printf("AOF stream successfully uploaded to S3: %s\n", location)
	uploadComplete <- nil
}
