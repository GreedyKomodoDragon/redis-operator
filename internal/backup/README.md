# Redis Backup Service

This Go service implements a robust Redis backup solution using the PSYNC protocol for continuous replication and backup.

## Features

### Core Functionality
- **PSYNC Protocol**: Connects to Redis primary node using PSYNC for full resynchronization
- **RDB Snapshots**: Captures and saves complete Redis database snapshots
- **AOF Streaming**: Continuously reads and logs command stream in append-only format
- **Connection Retry**: Robust retry logic with exponential backoff
- **IPv6 Support**: Proper address formatting for both IPv4 and IPv6

### Backup Features
- **Local Storage**: Saves RDB and AOF files to configurable backup directory
- **S3 Upload**: Optional upload to AWS S3 with proper error handling
- **File Cleanup**: Automatic cleanup of old backup files based on age and count
- **Graceful Reconnect**: Handles disconnections and restarts backup cycle

### Configuration

The service is configured through environment variables:

#### Redis Connection
- `REDIS_HOST`: Redis server hostname (default: localhost)
- `REDIS_PORT`: Redis server port (default: 6379)
- `REDIS_PASSWORD`: Redis password (optional)
- `REDIS_TLS_ENABLED`: Enable TLS connection (default: false)

#### Backup Settings
- `BACKUP_DIR`: Local backup directory (default: /data)
- `BACKUP_RETENTION_HOURS`: Keep backups for N hours (default: 24)
- `BACKUP_MAX_FILES`: Maximum number of backup files (default: 10)

#### Connection Retry
- `MAX_RETRIES`: Maximum connection retry attempts (default: 5)
- `RETRY_DELAY_SECONDS`: Initial retry delay (default: 5)
- `MAX_RETRY_DELAY_SECONDS`: Maximum retry delay (default: 300)

#### S3 Upload (Optional)
- `S3_BUCKET`: S3 bucket name for uploads
- `AWS_REGION`: AWS region (default: us-east-1)
- AWS credentials via standard AWS environment variables or IAM roles

## How It Works

### 1. Connection and Authentication
- Establishes TCP connection to Redis (with optional TLS)
- Authenticates using provided password if configured
- Implements retry logic with exponential backoff

### 2. PSYNC Protocol
- Sends `PSYNC ? -1` command to force full resynchronization
- Parses `+FULLRESYNC <replid> <offset>` response
- Extracts replication ID and offset for tracking

### 3. RDB Snapshot
- Reads RESP bulk string containing RDB data
- Saves complete snapshot to timestamped file
- Uploads to S3 if configured

### 4. Command Stream
- Continuously reads RESP arrays (Redis commands)
- Logs commands to append-only file (AOF format)
- Periodically syncs to disk for durability

### 5. Error Handling
- Graceful handling of connection failures
- Automatic reconnection and restart of backup cycle
- Cleanup of old backup files on restart

## File Structure

```
internal/backup/
├── service.go          # Main backup service and orchestration
├── redis_client.go     # Redis connection and PSYNC protocol handling
├── s3_upload.go        # S3 upload functionality
└── README.md           # Documentation
```

## Architecture

The backup service is now cleanly separated into distinct components:

### RedisClient (`redis_client.go`)
- Handles TCP/TLS connections to Redis
- Implements PSYNC protocol operations
- Provides RESP protocol parsing
- Manages connection retry logic
- Abstracted Redis operations for reusability

### BackupService (`service.go`)
- Orchestrates the overall backup process
- Manages local file operations (RDB/AOF)
- Handles S3 uploads and cleanup
- Coordinates backup lifecycle

### S3Uploader (`s3_upload.go`)
- AWS S3 upload functionality using AWS CLI
- Error handling and timeout management
- Proper S3 key organization

## Usage

### As a Standalone Service
```go
package main

import "github.com/GreedyKomodoDragon/redis-operator/internal/backup"

func main() {
    backup.Run()
}
```

### Using RedisClient Independently
```go
package main

import (
    "fmt"
    "github.com/GreedyKomodoDragon/redis-operator/internal/backup"
)

func main() {
    // Create Redis client
    client := backup.NewRedisClient("localhost", "6379", "", false)
    
    // Connect and authenticate
    if err := client.Connect(); err != nil {
        panic(err)
    }
    defer client.Close()
    
    if err := client.Authenticate(); err != nil {
        panic(err)
    }
    
    // Send PSYNC
    replInfo, err := client.SendPSYNC()
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Replication ID: %s, Offset: %d\n", replInfo.ID, replInfo.Offset)
    
    // Read RDB snapshot
    rdbData, err := client.ReadRDBSnapshot()
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("RDB size: %d bytes\n", len(rdbData))
}
```

### As Part of Operator
The service integrates with the Redis operator for automated backup scheduling and management.

## Implementation Details

### RESP Protocol Parsing
- Handles RESP arrays for command parsing
- Supports bulk strings for RDB data
- Proper handling of Redis protocol nuances

### S3 Integration
- Uses AWS CLI for reliable uploads
- Implements proper error handling and timeouts
- Organizes backups by replication ID

### File Management
- Timestamped file naming for easy identification
- Automatic cleanup based on age and count limits
- Proper file handling with error recovery

## Monitoring and Logging

The service provides detailed logging for:
- Connection status and retry attempts
- Backup progress and file sizes
- S3 upload status and errors
- Cleanup operations and statistics

## Production Considerations

1. **Resource Usage**: Monitor disk space for backup files
2. **Network**: Ensure stable connection to Redis primary
3. **AWS Credentials**: Use IAM roles for S3 access in production
4. **Monitoring**: Set up alerts for backup failures
5. **Testing**: Regularly test backup restore procedures
