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
	backupCount := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Backup service shutting down...")
			return ctx.Err()
		default:
			backupCount++
			fmt.Printf("Starting backup cycle #%d\n", backupCount)

			if err := s.performBackupCycle(); err != nil {
				fmt.Printf("Backup cycle #%d failed: %v\n", backupCount, err)

				// Try graceful reconnect
				if reconnectErr := s.gracefulReconnect(); reconnectErr != nil {
					fmt.Printf("Graceful reconnect failed: %v\n", reconnectErr)
				}

				fmt.Println("Retrying in 30 seconds...")
				time.Sleep(30 * time.Second)
				continue
			}

			fmt.Printf("Backup cycle #%d completed successfully\n", backupCount)

			// For continuous backup, wait before next cycle
			// In production, this might be controlled by a schedule
			fmt.Println("Waiting 5 minutes before next backup cycle...")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Minute):
				// Continue to next backup cycle
			}
		}
	}
}

func (s *BackupService) performBackupCycle() error {
	fmt.Println("Starting backup cycle...")

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
	fmt.Printf("Starting full resync - ID: %s, Offset: %d\n", s.replID, s.replOffset)

	// Step 4: Stream RDB snapshot directly to S3
	if err := s.streamRDBToS3(); err != nil {
		return fmt.Errorf("failed to stream RDB to S3: %v", err)
	}

	// Step 5: Stream command stream directly to S3
	if err := s.streamCommandsToS3(); err != nil {
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

func (s *BackupService) streamCommandsToS3() error {
	fmt.Println("Starting command stream backup to S3...")

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

	// Stream commands to the pipe with timeout and keepalive handling
	commandCount := 0
	defer writer.Close()

	// Use a shorter timeout to check for commands frequently
	commandTimeout := 30 * time.Second
	maxStreamDuration := 10 * time.Minute
	keepaliveInterval := 60 * time.Second

	streamStart := time.Now()
	lastKeepalive := time.Now()

	for {
		// Check if we've exceeded the maximum stream duration
		if time.Since(streamStart) > maxStreamDuration {
			fmt.Printf("Maximum stream duration (%v) reached - completing backup\n", maxStreamDuration)
			break
		}

		// Send keepalive if needed
		if time.Since(lastKeepalive) > keepaliveInterval {
			if err := s.redisClient.SendKeepAlive(); err != nil {
				fmt.Printf("Keepalive failed: %v - ending stream\n", err)
				break
			}
			lastKeepalive = time.Now()
			fmt.Println("Sent keepalive to maintain Redis connection")
		}

		// Try to read a command with timeout
		command, err := s.redisClient.ReadRESPCommandWithTimeout(commandTimeout)
		if err != nil {
			// Check if it's a timeout (which is expected with low traffic)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected with low traffic - continue waiting
				fmt.Printf("No commands received in %v - continuing to wait...\n", commandTimeout)
				continue
			}

			// Check for connection closed
			if err == io.EOF {
				fmt.Println("Redis connection closed - attempting to complete backup gracefully")
				break
			}

			// Other errors
			fmt.Printf("Error reading command: %v - ending stream\n", err)
			writer.CloseWithError(fmt.Errorf("failed to read command: %v", err))
			break
		}

		// Successfully read a command - write it to the pipe
		if _, err := writer.Write([]byte(command)); err != nil {
			writer.CloseWithError(fmt.Errorf("failed to write command: %v", err))
			break
		}

		commandCount++
		if commandCount%100 == 0 {
			fmt.Printf("Streamed %d commands...\n", commandCount)
		}
	}

	// Wait for upload to complete
	if err := <-uploadComplete; err != nil {
		return err
	}

	fmt.Printf("Command stream ended. Total commands processed: %d\n", commandCount)
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
