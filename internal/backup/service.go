package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type BackupService struct {
	backupDir   string
	s3Bucket    string
	s3Enabled   bool
	redisClient *RedisClient

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

	// Get backup configuration
	s.backupDir = getEnvOrDefault("BACKUP_DIR", "/data")

	// S3 configuration
	s.s3Bucket = os.Getenv("S3_BUCKET")
	s.s3Enabled = s.s3Bucket != ""

	// Ensure backup directory exists
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %v", err)
	}

	fmt.Printf("Backup service initialized - Redis: %s:%s, TLS: %v, Dir: %s, S3: %v\n",
		redisHost, redisPort, tlsEnabled, s.backupDir, s.s3Enabled)
	return nil
}

func (s *BackupService) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Backup service shutting down...")
			return ctx.Err()
		default:
			if err := s.performBackupCycle(); err != nil {
				fmt.Printf("Backup cycle failed: %v\n", err)

				// Try graceful reconnect
				if reconnectErr := s.gracefulReconnect(); reconnectErr != nil {
					fmt.Printf("Graceful reconnect failed: %v\n", reconnectErr)
				}

				fmt.Println("Retrying in 30 seconds...")
				time.Sleep(30 * time.Second)
				continue
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

	// Step 4: Read and save RDB snapshot
	rdbFile, err := s.readRDB()
	if err != nil {
		return fmt.Errorf("failed to read RDB: %v", err)
	}
	fmt.Printf("RDB snapshot saved to: %s\n", rdbFile)

	// Step 5: Upload RDB to S3 if enabled
	if s.s3Enabled {
		if err := s.uploadToS3(rdbFile); err != nil {
			fmt.Printf("Warning: Failed to upload RDB to S3: %v\n", err)
			// Don't fail the backup cycle for S3 upload failures
		}
	}

	// Step 6: Read command stream and save to AOF
	aofFile, err := s.readCommandStream()
	if err != nil {
		return fmt.Errorf("command stream failed: %v", err)
	}
	fmt.Printf("AOF stream saved to: %s\n", aofFile)

	// Step 7: Upload AOF to S3 if enabled
	if s.s3Enabled {
		if err := s.uploadToS3(aofFile); err != nil {
			fmt.Printf("Warning: Failed to upload AOF to S3: %v\n", err)
			// Don't fail the backup cycle for S3 upload failures
		}
	}

	return nil
}

func (s *BackupService) readRDB() (string, error) {
	// Use the RedisClient to read RDB snapshot
	rdbData, err := s.redisClient.ReadRDBSnapshot()
	if err != nil {
		return "", fmt.Errorf("failed to read RDB data: %v", err)
	}

	// Create RDB file
	timestamp := time.Now().Format("20060102-150405")
	rdbPath := filepath.Join(s.backupDir, fmt.Sprintf("dump-%s.rdb", timestamp))
	rdbFile, err := os.Create(rdbPath)
	if err != nil {
		return "", fmt.Errorf("failed to create RDB file: %v", err)
	}
	defer rdbFile.Close()

	// Write RDB data to file
	written, err := rdbFile.Write(rdbData)
	if err != nil {
		return "", fmt.Errorf("failed to write RDB data: %v", err)
	}

	fmt.Printf("RDB snapshot saved successfully (%d bytes)\n", written)
	return rdbPath, nil
}

func (s *BackupService) readCommandStream() (string, error) {
	// Create AOF file for command stream
	timestamp := time.Now().Format("20060102-150405")
	aofPath := filepath.Join(s.backupDir, fmt.Sprintf("appendonly-%s.aof", timestamp))
	aofFile, err := os.Create(aofPath)
	if err != nil {
		return "", fmt.Errorf("failed to create AOF file: %v", err)
	}
	defer aofFile.Close()

	fmt.Println("Starting to read command stream...")
	commandCount := 0

	for {
		// Read RESP commands from the replication stream using RedisClient
		command, err := s.redisClient.ReadRESPCommand()
		if err != nil {
			if err == io.EOF {
				fmt.Println("Redis connection closed")
				break
			}
			return "", fmt.Errorf("failed to read command: %v", err)
		}

		// Write command to AOF file
		if _, err := aofFile.WriteString(command); err != nil {
			return "", fmt.Errorf("failed to write to AOF: %v", err)
		}

		commandCount++
		if commandCount%100 == 0 {
			fmt.Printf("Processed %d commands...\n", commandCount)
			aofFile.Sync() // Flush to disk periodically
		}
	}

	fmt.Printf("Command stream ended. Total commands processed: %d\n", commandCount)
	return aofPath, nil
}

// uploadToS3 uploads a file to S3 bucket using AWS CLI
func (s *BackupService) uploadToS3(filePath string) error {
	if !s.s3Enabled {
		return nil
	}

	// Use the enhanced S3 upload function
	return s.uploadToS3Enhanced(filePath)
}

// gracefulReconnect handles reconnection and restart of backup cycle
func (s *BackupService) gracefulReconnect() error {
	fmt.Println("Attempting graceful reconnect...")
	s.redisClient.Close()

	// Clean up old files since we need a fresh full resync
	if err := s.cleanupOldBackups(); err != nil {
		fmt.Printf("Warning: Failed to cleanup old backups: %v\n", err)
	}

	time.Sleep(5 * time.Second) // Wait before reconnecting
	return nil
}

// cleanupOldBackups removes old backup files to save disk space
func (s *BackupService) cleanupOldBackups() error {
	maxAge := time.Duration(getEnvIntOrDefault("BACKUP_RETENTION_HOURS", 24)) * time.Hour
	maxFiles := getEnvIntOrDefault("BACKUP_MAX_FILES", 10)

	fmt.Printf("Cleaning up backups older than %v or keeping only %d files...\n", maxAge, maxFiles)

	files, err := s.getBackupFiles()
	if err != nil {
		return fmt.Errorf("failed to get backup files: %v", err)
	}

	filesToDelete := s.getFilesToDeleteByAge(files, maxAge)
	filesToDelete = append(filesToDelete, s.getFilesToDeleteByCount(files, filesToDelete, maxFiles)...)

	return s.deleteFiles(filesToDelete)
}

// getFilesToDeleteByAge returns files older than maxAge
func (s *BackupService) getFilesToDeleteByAge(files []string, maxAge time.Duration) []string {
	now := time.Now()
	var filesToDelete []string

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > maxAge {
			filesToDelete = append(filesToDelete, file)
		}
	}

	return filesToDelete
}

// getFilesToDeleteByCount returns oldest files to keep only maxFiles
func (s *BackupService) getFilesToDeleteByCount(files, alreadyDeleting []string, maxFiles int) []string {
	remaining := s.getRemainingFiles(files, alreadyDeleting)

	excess := len(remaining) - maxFiles
	if excess <= 0 {
		return []string{}
	}

	// Return the oldest files
	return remaining[:excess]
}

// getRemainingFiles returns files not already marked for deletion
func (s *BackupService) getRemainingFiles(files, alreadyDeleting []string) []string {
	var remaining []string

	for _, file := range files {
		if !s.containsString(alreadyDeleting, file) {
			remaining = append(remaining, file)
		}
	}

	return remaining
}

// deleteFiles removes the specified files
func (s *BackupService) deleteFiles(filesToDelete []string) error {
	for _, file := range filesToDelete {
		if err := os.Remove(file); err != nil {
			fmt.Printf("Warning: Failed to delete %s: %v\n", file, err)
		} else {
			fmt.Printf("Deleted old backup: %s\n", filepath.Base(file))
		}
	}
	return nil
}

// containsString checks if a slice contains a string
func (s *BackupService) containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getBackupFiles returns a list of all backup files in the backup directory
func (s *BackupService) getBackupFiles() ([]string, error) {
	files := []string{}

	err := filepath.Walk(s.backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Check if it's a backup file (RDB or AOF)
			if strings.HasSuffix(path, ".rdb") || strings.HasSuffix(path, ".aof") {
				files = append(files, path)
			}
		}

		return nil
	})

	return files, err
}

// Helper functions for environment variables
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Helper function for min
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
