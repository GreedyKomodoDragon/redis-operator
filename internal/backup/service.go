package backup

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type BackupService struct {
	redisClient *redis.Client
}

func Run() {
	// Initialize the backup service
	backupService := &BackupService{}

	// Initialize Redis client with retry
	maxRetries := 10
	waitTime := 5 * time.Second

	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("Attempt %d/%d: Initializing Redis connection...\n", attempt, maxRetries)

		err = backupService.initRedisClient()
		if err == nil {
			fmt.Printf("Successfully connected to Redis on attempt %d\n", attempt)
			break
		}

		fmt.Printf("Connection attempt %d failed: %v\n", attempt, err)
		if attempt < maxRetries {
			fmt.Printf("Waiting %v before retry...\n", waitTime)
			time.Sleep(waitTime)
		}
	}

	if err != nil {
		panic(fmt.Errorf("failed to initialize Redis client after %d attempts: %v", maxRetries, err))
	}

	defer backupService.redisClient.Close()

	// Start the backup service
	if err := backupService.Start(context.Background()); err != nil {
		panic(err)
	}
}

func (s *BackupService) initRedisClient() error {
	// Get Redis connection details from environment variables
	redisHost := getEnvOrDefault("REDIS_HOST", "localhost")
	redisPort := getEnvOrDefault("REDIS_PORT", "6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := getEnvIntOrDefault("REDIS_DB", 0)
	tlsEnabled := getEnvOrDefault("REDIS_TLS_ENABLED", "false") == "true"

	fmt.Printf("Attempting to connect to Redis at %s:%s (TLS: %v)\n", redisHost, redisPort, tlsEnabled)

	// Create Redis client options
	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       redisDB,
	}

	// Configure TLS if enabled
	if tlsEnabled {
		// For TLS connections, we'll use a basic TLS config
		// In production, you might want to configure certificates
		options.TLSConfig = &tls.Config{
			ServerName: redisHost,
			// InsecureSkipVerify: true, // Only for testing - don't use in production
		}
	}

	// Create Redis client
	s.redisClient = redis.NewClient(options)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	tlsStatus := ""
	if tlsEnabled {
		tlsStatus = " (TLS enabled)"
	}
	fmt.Printf("Successfully connected to Redis at %s:%s%s\n", redisHost, redisPort, tlsStatus)
	return nil
}

func (s *BackupService) Start(ctx context.Context) error {
	// timer and context setup - ping every 10 seconds for now
	timer := time.NewTicker(10 * time.Second)
	defer timer.Stop()

	fmt.Println("Backup service started, pinging Redis every 10 seconds...")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Backup service shutting down...")
			return ctx.Err()
		case <-timer.C:
			// Ping Redis to check connectivity
			if err := s.pingRedis(); err != nil {
				fmt.Printf("Redis ping failed: %v\n", err)
			} else {
				fmt.Println("Redis ping successful")
			}
		}
	}
}

func (s *BackupService) pingRedis() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping Redis
	result := s.redisClient.Ping(ctx)
	if err := result.Err(); err != nil {
		return fmt.Errorf("ping failed: %v", err)
	}

	// Get some basic Redis info
	info := s.redisClient.Info(ctx, "server")
	if err := info.Err(); err != nil {
		fmt.Printf("Warning: Could not get Redis info: %v\n", err)
	} else {
		// Extract Redis version from info (optional)
		infoStr := info.Val()
		fmt.Printf("Redis connection healthy. Info: %s\n", infoStr[:min(100, len(infoStr))])
	}

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

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
