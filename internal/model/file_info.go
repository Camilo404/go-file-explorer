package model

import "time"

type FileItem struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Type         string    `json:"type"`
	Size         int64     `json:"size"`
	SizeHuman    string    `json:"size_human,omitempty"`
	MimeType     string    `json:"mime_type,omitempty"`
	Extension    string    `json:"extension,omitempty"`
	PreviewURL   string    `json:"preview_url,omitempty"`
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
	IsImage      bool      `json:"is_image,omitempty"`
	IsVideo      bool      `json:"is_video,omitempty"`
	ModifiedAt   time.Time `json:"modified_at"`
	CreatedAt    time.Time `json:"created_at"`
	MatchContext string    `json:"match_context,omitempty"`
	Permissions  string    `json:"permissions"`
	ItemCount    *int      `json:"item_count,omitempty"`
}

type DirectoryListData struct {
	CurrentPath string     `json:"current_path"`
	ParentPath  string     `json:"parent_path"`
	Items       []FileItem `json:"items"`
}

type DirectoryCreateData struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}
