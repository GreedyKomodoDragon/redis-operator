package initbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

func (s *Service) Run(dataDir string) error {
	if dataDir == "" {
		dataDir = "/data" // Default Redis data directory
	}

	s.logger.Info("Starting init-backup service", "data_dir", dataDir)

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

	// Download and restore the backup files
	if err := s.restoreBackupSet(backupSet, dataDir); err != nil {
		s.logger.Error("Failed to restore backup set", "error", err)
		return err
	}

	s.logger.Info("Backup restoration completed successfully")
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

// restoreBackupSet downloads and restores the RDB and AOF files from the backup set
func (s *Service) restoreBackupSet(backupSet *BackupSet, dataDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	s.logger.Info("Starting backup restoration", "data_dir", dataDir)

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory %s: %v", dataDir, err)
	}

	// Clean up any existing Redis data files to avoid permission issues
	s.cleanupExistingRedisFiles(dataDir)

	// Download and restore RDB file
	if err := s.downloadAndSaveRDBFile(ctx, backupSet.RDBFile, dataDir); err != nil {
		return fmt.Errorf("failed to restore RDB file: %v", err)
	}

	// Download and restore AOF files
	if len(backupSet.AOFFiles) > 0 {
		if err := s.downloadAndSaveAOFFiles(ctx, backupSet.AOFFiles, dataDir); err != nil {
			return fmt.Errorf("failed to restore AOF files: %v", err)
		}
	} else {
		s.logger.Info("No AOF files to restore")
	}

	return nil
}

// downloadAndSaveRDBFile downloads the RDB file and saves it as dump.rdb
func (s *Service) downloadAndSaveRDBFile(ctx context.Context, rdbFile *RDBFile, dataDir string) error {
	s.logger.Info("Downloading RDB file", "key", rdbFile.Key, "size", rdbFile.Size)

	// Save as dump.rdb (Redis default name)
	rdbPath := filepath.Join(dataDir, "dump.rdb")

	// Create the file
	file, err := os.Create(rdbPath)
	if err != nil {
		return fmt.Errorf("failed to create RDB file %s: %v", rdbPath, err)
	}
	defer file.Close()

	// Stream the file directly to disk for memory efficiency
	if err := s.objectStore.DownloadFileToWriter(ctx, rdbFile.Key, file); err != nil {
		return fmt.Errorf("failed to download RDB file %s: %v", rdbFile.Key, err)
	}

	// Get file info for verification
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %v", rdbPath, err)
	}

	s.logger.Info("RDB file restored successfully",
		"path", rdbPath,
		"size", fileInfo.Size(),
		"original_key", rdbFile.Key)

	return nil
}

// downloadAndSaveAOFFiles downloads AOF files and saves them appropriately
func (s *Service) downloadAndSaveAOFFiles(ctx context.Context, aofFiles []AOFFile, dataDir string) error {
	s.logger.Info("Downloading AOF files", "count", len(aofFiles))

	for i, aofFile := range aofFiles {
		s.logger.Info("Downloading AOF file", "index", i+1, "total", len(aofFiles), "key", aofFile.Key)

		// Create the AOF file
		var aofPath string
		if len(aofFiles) == 1 {
			aofPath = filepath.Join(dataDir, "appendonly.aof")
		} else {
			// For multiple AOF files, use appendonly.aof.1, appendonly.aof.2, etc.
			aofPath = filepath.Join(dataDir, fmt.Sprintf("appendonly.aof.%d", i+1))
		}

		// Remove existing file if it exists to avoid permission issues
		if err := os.Remove(aofPath); err != nil && !os.IsNotExist(err) {
			s.logger.Debug("Failed to remove existing AOF file", "path", aofPath, "error", err)
			// Continue anyway, as os.Create might still work
		}

		file, err := os.Create(aofPath)
		if err != nil {
			return fmt.Errorf("failed to create AOF file %s: %v", aofPath, err)
		}

		// Stream the file directly to disk for memory efficiency
		if err := s.objectStore.DownloadFileToWriter(ctx, aofFile.Key, file); err != nil {
			file.Close()
			return fmt.Errorf("failed to download AOF file %s: %v", aofFile.Key, err)
		}

		// Get file info for verification
		fileInfo, err := file.Stat()
		file.Close()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %v", aofPath, err)
		}

		s.logger.Info("AOF file restored successfully",
			"path", aofPath,
			"size", fileInfo.Size(),
			"original_key", aofFile.Key)
	}

	return nil
}

// cleanupExistingRedisFiles removes existing Redis data files that might have permission issues
func (s *Service) cleanupExistingRedisFiles(dataDir string) {
	// List of files and directories to clean up
	cleanupPaths := []string{
		filepath.Join(dataDir, "dump.rdb"),
		filepath.Join(dataDir, "appendonly.aof"),
		filepath.Join(dataDir, "appendonlydir"),
	}

	for _, path := range cleanupPaths {
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			s.logger.Debug("Failed to remove existing Redis file/directory",
				"path", path, "error", err)
			// Continue anyway - we'll try to overwrite
		} else if err == nil {
			s.logger.Debug("Removed existing Redis file/directory", "path", path)
		}
	}
}
