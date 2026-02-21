package model

// TrashRecord represents a soft-deleted file/directory tracked in the trash system.
type TrashRecord struct {
	ID           string     `json:"id"`
	OriginalPath string     `json:"original_path"`
	TrashName    string     `json:"trash_name"`
	DeletedAt    string     `json:"deleted_at"`
	DeletedBy    AuditActor `json:"deleted_by"`
	RestoredAt   string     `json:"restored_at,omitempty"`
	RestoredBy   AuditActor `json:"restored_by,omitempty"`
}
