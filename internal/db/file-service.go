package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TinkerUp/adb-server/types/models"
)

type FileService interface {
	SaveFile(owner string, accessGroups []string, filename string, data []byte) (models.FileIndex, error)
	UpdateFile(owner string, accessGroups []string, fileId string, filename string, data []byte) (models.FileIndex, error)

	GetFile(owner string, fileId string) (models.File, error)
	ListFiles(owner string) ([]models.FileIndex, error)

	DeleteFile(owner string, fileId string) error
}

type fileService struct {
	root string
	db   *sql.DB
}

func NewFileService(root string, db *sql.DB) *fileService {
	return &fileService{
		root,
		db,
	}
}

func (s *fileService) SaveFile(owner string, accessGroups []string, filename string, data []byte) (models.FileIndex, error) {
	cleanName := filepath.Base(filename)
	sandBoxRoot := filepath.Join(s.root, owner)

	if err := os.MkdirAll(sandBoxRoot, DirPermsDefault); err != nil {
		return models.FileIndex{}, fmt.Errorf("failed to create directory: %w", err)
	}

	checksumHash := sha256.Sum256(data)
	checksum := hex.EncodeToString(checksumHash[:])

	nameHash := sha256.Sum256([]byte(cleanName + owner + time.Now().String()))
	fileId := hex.EncodeToString(nameHash[:])

	fileName := cleanName + "-" + fileId
	absFilePath := filepath.Join(sandBoxRoot, fileName)
	cleanFilePath := filepath.Clean(absFilePath)

	if !strings.HasPrefix(cleanFilePath, s.root) {
		return models.FileIndex{}, fmt.Errorf("file path escapes root: %s", cleanFilePath)
	}

	if err := os.WriteFile(absFilePath, data, FilePermsDefault); err != nil {
		return models.FileIndex{}, fmt.Errorf("failed to write file: %w", err)
	}

	fileIndex := models.FileIndex{
		ID:           fileId,
		Size:         int64(len(data)),
		Owner:        owner,
		Checksum:     checksum,
		FilePath:     cleanFilePath,
		CreatedAt:    time.Now().Unix(),
		AccessGroups: accessGroups,
	}

	if _, err := s.db.Exec(
		"INSERT INTO file_index (id, ownerId, size, filepath, createdAt, checksum, accessGroups) VALUES (?, ?, ?, ?, ?, ?, ?)",
		fileIndex.ID, fileIndex.Owner, fileIndex.Size, fileIndex.FilePath, fileIndex.CreatedAt, fileIndex.Checksum, fileIndex.AccessGroups,
	); err != nil {
		return models.FileIndex{}, fmt.Errorf("failed to insert file index: %w", err)
	}

	return fileIndex, nil
}

func (s *fileService) GetFile(owner string, fileId string) (models.File, error) {
	fileIndex, err := s.db.Query(
		"Select id, ownerId, size, filepath, createdAt, checksum, accessGroups FROM file_index WHERE id = ? AND ownerId = ?",
		fileId, owner,
	)

	if err != nil {
		return models.File{}, fmt.Errorf("failed to query file index: %w", err)
	}

	defer fileIndex.Close()

	var metadata models.FileIndex

	if err := fileIndex.Scan(
		&metadata.ID, &metadata.Owner, &metadata.Size, &metadata.FilePath, &metadata.CreatedAt, &metadata.Checksum, &metadata.AccessGroups,
	); err != nil {
		return models.File{}, fmt.Errorf("failed to scan file index: %w", err)
	}

	filePath := metadata.FilePath

	if !strings.HasPrefix(filePath, s.root) {
		return models.File{}, fmt.Errorf("file path escapes root: %s", filePath)
	}

	data, err := os.ReadFile(filePath)

	if err != nil {
		return models.File{}, fmt.Errorf("failed to read file: %w", err)
	}

	fileHash := sha256.Sum256(data)
	checkSum := hex.EncodeToString(fileHash[:])

	if metadata.Checksum != checkSum || metadata.Size != int64(len(data)) {
		return models.File{}, fmt.Errorf("file checksum or size mismatch")
	}

	return models.File{
		Metadata: metadata,
		Data:     data,
	}, nil
}

const (
	// Directories
	DirPermsDefault os.FileMode = 0o700 // rwx------
	DirPermsRelaxed os.FileMode = 0o755 // rwxr-xr-x

	// Files
	FilePermsDefault  os.FileMode = 0o600 // rw-------
	FilePermsRelaxed  os.FileMode = 0o644 // rw-r--r--
	FilePermsReadOnly os.FileMode = 0o400 // r--------
)
