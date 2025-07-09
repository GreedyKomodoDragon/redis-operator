package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"
)

type BackupService struct {
	S3Bucket    string
	S3Enabled   bool
	redisClient *RedisClient
	logger      *slog.Logger

	// New object-oriented components
	backupManager *BackupObjectManager

	// Backup retention settings
	retentionCount int

	// Replication state
	ReplID     string
	replOffset int64
}

type ReplicationInfo struct {
	ID     string
	Offset int64
}

func Run() {
	// Initialize structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Initialize the backup service
	backupService := &BackupService{
		logger: logger,
	}

	if err := backupService.initialize(); err != nil {
		logger.Error("Failed to initialize backup service", "error", err)
		os.Exit(1)
	}

	// Start the backup process
	if err := backupService.Start(context.Background()); err != nil {
		logger.Error("Backup service failed", "error", err)
		os.Exit(1)
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

	// Backup retention configuration
	s.retentionCount = getEnvIntOrDefault("BACKUP_RETENTION", 7) // Default to keeping 7 backups

	// S3 configuration - required for backup
	s.S3Bucket = os.Getenv("S3_BUCKET")
	s.S3Enabled = s.S3Bucket != ""

	if !s.S3Enabled {
		return fmt.Errorf("S3_BUCKET environment variable is required - backup service only supports S3 storage")
	}

	// Initialize S3 uploader
	s3Config := S3Config{
		Bucket:          s.S3Bucket,
		Region:          getEnvOrDefault("AWS_REGION", "us-east-1"),
		Endpoint:        os.Getenv("AWS_ENDPOINT_URL"),
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create S3 client
	s3Client, err := NewS3Client(ctx, s3Config)
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %v", err)
	}

	// Initialize new object-oriented backup manager
	uploader := NewS3Uploader(ctx, s3Client, s3Config.Bucket)
	retentionManager := NewS3RetentionManager(s3Client, s3Config.Bucket, s.logger)
	s.backupManager = NewBackupObjectManager(uploader, retentionManager, s.logger)

	s.logger.Info("Backup service initialized",
		"redis_host", redisHost,
		"redis_port", redisPort,
		"tls_enabled", tlsEnabled,
		"s3_enabled", s.S3Enabled,
		"s3_bucket", s.S3Bucket,
	)
	return nil
}

func (s *BackupService) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Backup service shutting down")
			return ctx.Err()
		default:
			s.logger.Info("Starting backup service as Redis replica")

			if err := s.runAsReplica(ctx); err != nil {
				s.logger.Error("Replica connection failed", "error", err)

				// Try graceful reconnect
				if reconnectErr := s.gracefulReconnect(); reconnectErr != nil {
					s.logger.Error("Graceful reconnect failed", "error", reconnectErr)
				}

				s.logger.Info("Retrying replica connection", "retry_delay", "30s")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(30 * time.Second):
					continue
				}
			}

			s.logger.Info("Replica connection ended, attempting to reconnect")
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

	s.ReplID = replInfo.ID
	s.replOffset = replInfo.Offset
	s.logger.Info("Acting as Redis replica",
		"replication_id", s.ReplID,
		"replication_offset", s.replOffset,
	)

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
	s.logger.Info("Streaming RDB snapshot directly to S3")

	// Read RDB data from Redis
	rdbData, err := s.redisClient.ReadRDBSnapshot()
	if err != nil {
		return fmt.Errorf("failed to read RDB data: %v", err)
	}

	// Create a reader from the RDB data
	reader := bytes.NewReader(rdbData)

	// Generate S3 key
	timestamp := time.Now().Format("20060102-150405")
	s3Key := fmt.Sprintf("redis-backups/%s/dump-%s.rdb", s.ReplID, timestamp)

	// Metadata for the backup
	metadata := map[string]string{
		"uploaded-by":        "redis-operator",
		"timestamp":          fmt.Sprintf("%d", time.Now().Unix()),
		"replication-id":     s.ReplID,
		"replication-offset": fmt.Sprintf("%d", s.replOffset),
		"backup-type":        "rdb",
	}

	// Upload stream to S3 with progress tracking and retention management
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	progressCallback := func(bytes int64) {
		if bytes%(1024*1024) == 0 { // Log every MB
			s.logger.Debug("RDB upload progress", "bytes_uploaded", bytes)
		}
	}

	err = s.backupManager.UploadWithProgressAndRetention(ctx, reader, s3Key, metadata, s.ReplID, s.retentionCount, progressCallback)
	if err != nil {
		return fmt.Errorf("failed to upload RDB to S3: %v", err)
	}

	s.logger.Info("RDB snapshot successfully uploaded to S3",
		"size_bytes", len(rdbData),
		"s3_key", s3Key,
	)

	return nil
}

func (s *BackupService) streamCommandsToS3(ctx context.Context) error {
	s.logger.Info("Starting persistent command stream backup to S3")

	// Create a pipe for streaming commands
	reader, writer := io.Pipe()

	// Generate S3 key
	timestamp := time.Now().Format("20060102-150405")
	s3Key := fmt.Sprintf("redis-backups/%s/appendonly-%s.aof", s.ReplID, timestamp)

	// Metadata for the backup
	metadata := map[string]string{
		"uploaded-by":        "redis-operator",
		"timestamp":          fmt.Sprintf("%d", time.Now().Unix()),
		"replication-id":     s.ReplID,
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

	s.logger.Info("Acting as Redis replica - continuously waiting for commands",
		"command_timeout", commandTimeout,
		"keepalive_interval", keepaliveInterval,
	)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Context cancelled, stopping replica")
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
				s.logger.Debug("Replication progress", "commands_replicated", commandCount)
			}
		}
	}
}

// handleUploadComplete handles the completion of S3 upload
func (s *BackupService) handleUploadComplete(err error) error {
	if err != nil {
		s.logger.Error("S3 upload failed", "error", err)
		return err
	}
	s.logger.Info("S3 upload completed successfully")
	return nil
}

// shouldSendKeepalive checks if keepalive should be sent
func (s *BackupService) shouldSendKeepalive(lastKeepalive time.Time, interval time.Duration) bool {
	if time.Since(lastKeepalive) > interval {
		s.logger.Debug("Sending replication keepalive (REPLCONF ACK)")

		// Send REPLCONF ACK to maintain replication connection
		// This is the proper way to keepalive a replication connection
		if err := s.sendReplconfAck(); err != nil {
			s.logger.Warn("Failed to send REPLCONF ACK", "error", err)
		} else {
			s.logger.Debug("REPLCONF ACK sent successfully")
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
		s.logger.Warn("Master closed replication connection (EOF)")
		s.logger.Info("Common causes for connection closure:",
			"reasons", []string{
				"Redis master timeout for idle connections",
				"Network timeout/firewall rules",
				"Redis restart or configuration changes",
				"Maximum replica connections reached",
			},
		)
		return nil
	}

	// Check for specific network errors
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			s.logger.Warn("Network timeout during replication", "error", err)
		} else {
			s.logger.Error("Network error during replication", "error", err)
		}
	} else {
		s.logger.Error("Replication error", "error", err)
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
		s.logger.Error("Failed to write command to backup stream", "error", err)
		return err
	}
	return nil
}

// gracefulReconnect handles reconnection and restart of backup cycle
func (s *BackupService) gracefulReconnect() error {
	s.logger.Info("Attempting graceful reconnect")

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

	s.logger.Info("Successfully reconnected to Redis")
	s.redisClient.Close() // Close for now, will reconnect in next cycle
	return nil
}

// isValidTimestamp checks if a timestamp string matches the expected format: YYYYMMDD-HHMMSS
func isValidTimestamp(timestamp string) bool {
	// Expected format: 20240101-120000
	if len(timestamp) != 15 {
		return false
	}

	// Check for dash in the right position
	if timestamp[8] != '-' {
		return false
	}

	// Check if the date and time parts are numeric
	datePart := timestamp[:8] // YYYYMMDD
	timePart := timestamp[9:] // HHMMSS

	for _, r := range datePart {
		if r < '0' || r > '9' {
			return false
		}
	}

	for _, r := range timePart {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
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

	progressCallback := func(bytes int64) {
		if bytes%(1024*1024) == 0 { // Log every MB
			s.logger.Debug("AOF upload progress", "bytes_uploaded", bytes)
		}
	}

	err := s.backupManager.UploadWithProgressAndRetention(ctx, reader, s3Key, metadata, s.ReplID, s.retentionCount, progressCallback)
	if err != nil {
		uploadComplete <- fmt.Errorf("failed to upload AOF to S3: %v", err)
		return
	}

	s.logger.Info("AOF stream successfully uploaded to S3",
		"s3_key", s3Key,
	)
	uploadComplete <- nil
}
