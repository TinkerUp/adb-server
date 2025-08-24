package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TinkerUp/adb-server/types/models"
	"github.com/google/uuid"
)

type FileService interface {
	SaveFile(owner string, accessGroups []string, filename string, fileExtension string, data []byte) (models.FileIndex, error)
	UpdateFile(owner string, accessGroups []string, fileId string, filename string, data []byte) (models.FileIndex, error)

	GetFile(owner string, fileId string) (models.File, error)
	ListFiles(owner string) ([]models.FileIndex, error)

	DeleteFile(owner string, fileId string) error
}

type fileService struct {
	config FileServiceConfig
	db     *sql.DB
}

type FileServiceConfig struct {
	Root                  string
	SecureMode            bool
	AllowedFileExtensions []string
}

func NewFileService(config FileServiceConfig, db *sql.DB) *fileService {
	return &fileService{
		config,
		db,
	}
}

func (s *fileService) SaveFile(owner string, accessGroups []string, filename string, fileExtension string, data []byte) (models.FileIndex, error) {
	sandBoxRoot := filepath.Join(s.config.Root, owner)

	if err := os.MkdirAll(sandBoxRoot, DirPermsDefault); err != nil {
		return models.FileIndex{}, fmt.Errorf("failed to create directory: %w", err)
	}

	fileExtension = strings.ToLower(strings.TrimSpace(fileExtension))

	if !s.validateFileExtension(fileExtension) {
		return models.FileIndex{}, fmt.Errorf("file extension not allowed: %s", fileExtension)
	}

	sha256Sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sha256Sum[:])

	cleanFileName := filepath.Base(filename)

	fileId := uuid.NewString()

	fileName := fmt.Sprintf("%s-%s.%s", cleanFileName, fileId, fileExtension)
	absFilePath := filepath.Join(sandBoxRoot, fileName)
	cleanFilePath := filepath.Clean(absFilePath)

	if !strings.HasPrefix(cleanFilePath, s.config.Root) {
		return models.FileIndex{}, fmt.Errorf("file path escapes root: %s", cleanFilePath)
	}

	if err := os.WriteFile(absFilePath, data, FilePermsDefault); err != nil {
		return models.FileIndex{}, fmt.Errorf("failed to write file: %w", err)
	}

	fileIndex := models.FileIndex{
		ID:        fileId,
		Size:      int64(len(data)),
		Owner:     owner,
		FilePath:  cleanFilePath,
		Checksum:  checksum,
		CreatedAt: time.Now().Unix(),
	}

	for group := range accessGroups {
		_, err := s.db.Exec("INSERT INTO access_groups (fileId, groupId) VALUES (?, ?)", fileId, group)

		if err != nil {
			return models.FileIndex{}, fmt.Errorf("failed to insert access group: %w", err)
		}
	}

	if _, err := s.db.Exec(
		"INSERT INTO file_index (id, ownerId, size, filepath, createdAt, checksum) VALUES (?, ?, ?, ?, ?, ?)",
		fileIndex.ID, fileIndex.Owner, fileIndex.Size, fileIndex.FilePath, fileIndex.CreatedAt, fileIndex.Checksum,
	); err != nil {
		return models.FileIndex{}, fmt.Errorf("failed to insert file index: %w", err)
	}

	return fileIndex, nil
}

func (s *fileService) GetFile(ctx context.Context, owner string, fileId string) (models.File, error) {
	var metadata models.FileIndex

	fileErr := s.db.QueryRowContext(
		ctx,
		"Select id, ownerId, size, filepath, createdAt, checksum, accessGroups FROM file_index WHERE id = ? AND ownerId = ?",
		fileId, owner,
	).Scan(
		&metadata.ID, &metadata.Owner, &metadata.Size, &metadata.FilePath, &metadata.CreatedAt, &metadata.Checksum, &metadata.AccessGroups,
	)

	if fileErr == sql.ErrNoRows {
		return models.File{}, fmt.Errorf("file not found")
	} else if fileErr != nil {
		return models.File{}, fmt.Errorf("failed to query file index: %w", fileErr)
	}

	filePath := metadata.FilePath

	if !strings.HasPrefix(filePath, s.config.Root) {
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

func (s *fileService) validateFileExtension(fileExtension string) bool {
	for _, extension := range s.config.AllowedFileExtensions {
		if extension == fileExtension {
			return true
		}
	}
	return false
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
