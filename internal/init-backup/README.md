# Init-Backup Service

The init-backup service is responsible for discovering and preparing Redis backup files for restoration. It works in conjunction with the backup service to provide a complete backup and restore solution.

## Features

- **Backup Discovery**: Automatically finds the latest RDB file in S3
- **AOF Matching**: Locates AOF files that match the latest RDB backup
- **S3 Integration**: Works with both AWS S3 and S3-compatible storage (MinIO)
- **Structured Logging**: Uses Go's slog for consistent, structured logging

## How It Works

1. **Connect to S3**: Establishes connection using AWS SDK v2
2. **List Backup Files**: Scans the `redis-backups/` prefix for backup files
3. **Parse File Names**: Extracts replication ID and timestamp from file names
4. **Find Latest RDB**: Sorts RDB files by modification time and selects the newest
5. **Match AOF Files**: Finds AOF files with matching replication ID and timestamp

## File Naming Convention

The service expects backup files to follow this naming convention:

```
redis-backups/{replication_id}/dump-{timestamp}.rdb        # RDB snapshot
redis-backups/{replication_id}/appendonly-{timestamp}.aof  # AOF commands
```

Where:
- `replication_id`: Redis replication ID from PSYNC command
- `timestamp`: Format YYYYMMDD-HHMMSS

Example:
```
redis-backups/abc123def456/dump-20241225-120000.rdb
redis-backups/abc123def456/appendonly-20241225-120000.aof
```

## Environment Variables

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `S3_BUCKET` | Yes | S3 bucket name | `redis-backups` |
| `AWS_REGION` | No | AWS region | `us-east-1` (default) |
| `AWS_ENDPOINT_URL` | No | Custom S3 endpoint | `http://localhost:9000` (MinIO) |
| `AWS_ACCESS_KEY_ID` | Yes* | AWS access key | `AKIAIOSFODNN7EXAMPLE` |
| `AWS_SECRET_ACCESS_KEY` | Yes* | AWS secret key | `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` |

*Required unless using IAM roles or other AWS credential providers

## Usage

### Build and Run

```bash
# Build the service
go build -o bin/init-backup ./cmd/init-backup/

# Set environment variables
export S3_BUCKET="redis-backups"
export AWS_REGION="us-east-1"
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"

# Run the service
./bin/init-backup
```
## Future Enhancements

- **Download Files**: Download RDB and AOF files to local storage
- **Redis Restoration**: Load RDB file and replay AOF commands into Redis
- **Validation**: Verify file integrity and compatibility
- **Point-in-Time Recovery**: Support for restoring to specific timestamps
- **Compression**: Support for compressed backup files

## Testing

```bash
# Run unit tests
go test ./internal/init-backup/

# Run with verbose output
go test -v ./internal/init-backup/
```
