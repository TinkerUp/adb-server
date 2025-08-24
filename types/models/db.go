package models

type FileIndex struct {
	ID        string `json:"id"`
	Size      int64  `json:"size"`
	Owner     string `json:"owner"`
	FilePath  string `json:"file_path"`
	Checksum  string `json:"checksum"`
	CreatedAt int64  `json:"created_at"`
}

type File struct {
	Metadata FileIndex
	Data     []byte
}

type AccessGroupEntry struct {
	FileId  string `json:"file_id"`
	GroupId string `json:"group_id"`
}
