package initbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"
)

type Service struct {
	objectStore ObjectStore
	logger      *slog.Logger
}

type RDBFile struct {
	Key           string
	LastModified  time.Time
	Size          int64
	ReplicationID string
	Timestamp     string
}

type AOFFile struct {
	Key           string
	LastModified  time.Time
	Size          int64
	ReplicationID string
	Timestamp     string
}

type BackupSet struct {
	RDBFile  *RDBFile
	AOFFiles []AOFFile
}

func NewService(objectStore ObjectStore) *Service {
	// Initialize structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	return &Service{
		objectStore: objectStore,
		logger:      logger,
	}
}

func (s *Service) Run() error {
	s.logger.Info("Starting init-backup service")

	// Fetch the latest backup set (RDB + matching AOF files)
	backupSet, err := s.fetchLatestBackupSet()
	if err != nil {
		s.logger.Error("Failed to fetch latest backup set", "error", err)
		return err
	}

	if backupSet == nil || backupSet.RDBFile == nil {
		s.logger.Info("No backup sets found in object store")
		return nil
	}

	s.logger.Info("Latest backup set found",
		"rdb_key", backupSet.RDBFile.Key,
		"rdb_size", backupSet.RDBFile.Size,
		"rdb_timestamp", backupSet.RDBFile.Timestamp,
		"aof_files_count", len(backupSet.AOFFiles),
	)

	// Log details about AOF files
	for i, aofFile := range backupSet.AOFFiles {
		s.logger.Info("AOF file found",
			"index", i,
			"key", aofFile.Key,
			"size", aofFile.Size,
			"timestamp", aofFile.Timestamp,
		)
	}

	// For now, we only discover the latest RDB file
	// Future implementation will download and restore the RDB file
	s.logger.Info("RDB file discovery completed successfully")
	return nil
}

func (s *Service) fetchLatestBackupSet() (*BackupSet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	s.logger.Info("Searching for backup files in object store", "bucket", s.objectStore.GetBucketName())

	rdbFiles, aofFiles, err := s.listBackupFiles(ctx)
	if err != nil {
		return nil, err
	}

	if len(rdbFiles) == 0 {
		s.logger.Info("No RDB files found in object store")
		return nil, nil
	}

	// Sort RDB files by last modified time (newest first)
	sort.Slice(rdbFiles, func(i, j int) bool {
		return rdbFiles[i].LastModified.After(rdbFiles[j].LastModified)
	})

	latestRDB := &rdbFiles[0]

	s.logger.Info("Found backup files",
		"rdb_count", len(rdbFiles),
		"aof_count", len(aofFiles),
		"latest_rdb", latestRDB.Key,
	)

	// Find AOF files that match the latest RDB's replication ID and timestamp
	matchingAOFs := s.findMatchingAOFFiles(latestRDB, aofFiles)

	return &BackupSet{
		RDBFile:  latestRDB,
		AOFFiles: matchingAOFs,
	}, nil
}

func (s *Service) listBackupFiles(ctx context.Context) ([]RDBFile, []AOFFile, error) {
	files, err := s.objectStore.ListFiles(ctx, "redis-backups/")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list files from object store: %v", err)
	}

	var rdbFiles []RDBFile
	var aofFiles []AOFFile

	for _, file := range files {
		if strings.HasSuffix(file.Key, ".rdb") {
			rdbFile, err := s.parseRDBFile(file.Key, file.LastModified, file.Size)
			if err != nil {
				s.logger.Warn("Failed to parse RDB file", "key", file.Key, "error", err)
				continue
			}
			rdbFiles = append(rdbFiles, *rdbFile)
		} else if strings.HasSuffix(file.Key, ".aof") {
			aofFile, err := s.parseAOFFile(file.Key, file.LastModified, file.Size)
			if err != nil {
				s.logger.Warn("Failed to parse AOF file", "key", file.Key, "error", err)
				continue
			}
			aofFiles = append(aofFiles, *aofFile)
		}
	}

	return rdbFiles, aofFiles, nil
}

func (s *Service) parseRDBFile(key string, lastModified time.Time, size int64) (*RDBFile, error) {
	// Expected format: redis-backups/{replication_id}/dump-{timestamp}.rdb
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid RDB file path format: %s", key)
	}

	replicationID := parts[1]
	filename := parts[2]

	// Extract timestamp from filename: dump-{timestamp}.rdb
	if !strings.HasPrefix(filename, "dump-") || !strings.HasSuffix(filename, ".rdb") {
		return nil, fmt.Errorf("invalid RDB filename format: %s", filename)
	}

	timestamp := strings.TrimPrefix(filename, "dump-")
	timestamp = strings.TrimSuffix(timestamp, ".rdb")

	return &RDBFile{
		Key:           key,
		LastModified:  lastModified,
		Size:          size,
		ReplicationID: replicationID,
		Timestamp:     timestamp,
	}, nil
}

func (s *Service) parseAOFFile(key string, lastModified time.Time, size int64) (*AOFFile, error) {
	// Expected format: redis-backups/{replication_id}/appendonly-{timestamp}.aof
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid AOF file path format: %s", key)
	}

	replicationID := parts[1]
	filename := parts[2]

	// Extract timestamp from filename: appendonly-{timestamp}.aof
	if !strings.HasPrefix(filename, "appendonly-") || !strings.HasSuffix(filename, ".aof") {
		return nil, fmt.Errorf("invalid AOF filename format: %s", filename)
	}

	timestamp := strings.TrimPrefix(filename, "appendonly-")
	timestamp = strings.TrimSuffix(timestamp, ".aof")

	return &AOFFile{
		Key:           key,
		LastModified:  lastModified,
		Size:          size,
		ReplicationID: replicationID,
		Timestamp:     timestamp,
	}, nil
}

func (s *Service) findMatchingAOFFiles(rdbFile *RDBFile, aofFiles []AOFFile) []AOFFile {
	var matchingAOFs []AOFFile

	for _, aofFile := range aofFiles {
		// AOF files should have the same replication ID as the RDB file
		if aofFile.ReplicationID == rdbFile.ReplicationID {
			// For now, we'll include AOF files with timestamps close to the RDB timestamp
			// In a more sophisticated implementation, we might check if the AOF timestamp
			// is from the same backup session as the RDB
			if aofFile.Timestamp == rdbFile.Timestamp {
				matchingAOFs = append(matchingAOFs, aofFile)
			}
		}
	}

	// Sort AOF files by timestamp to maintain order
	sort.Slice(matchingAOFs, func(i, j int) bool {
		return matchingAOFs[i].Timestamp < matchingAOFs[j].Timestamp
	})

	s.logger.Debug("Found matching AOF files",
		"rdb_replication_id", rdbFile.ReplicationID,
		"rdb_timestamp", rdbFile.Timestamp,
		"matching_aof_count", len(matchingAOFs),
	)

	return matchingAOFs
}
