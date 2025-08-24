package models

type FileIndex struct {
	ID           string   `json:"id"`
	Size         int64    `json:"size"`
	Owner        string   `json:"owner"`
	FilePath     string   `json:"file_path"`
	CreatedAt    int64    `json:"created_at"`
	LastModified int64    `json:"last_modified"`
	AccessGroups []string `json:"access_groups"`
}
