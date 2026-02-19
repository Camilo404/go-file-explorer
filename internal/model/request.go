package model

type CreateDirectoryRequest struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type UploadFailure struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type UploadItem struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

type UploadResponse struct {
	Uploaded []UploadItem    `json:"uploaded"`
	Failed   []UploadFailure `json:"failed"`
}

type RenameRequest struct {
	Path    string `json:"path"`
	NewName string `json:"new_name"`
}

type RenameResponse struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Name    string `json:"name"`
}

type MoveRequest struct {
	Sources        []string `json:"sources"`
	Destination    string   `json:"destination"`
	ConflictPolicy string   `json:"conflict_policy,omitempty"`
}

type MoveCopyResult struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type MoveCopyFailure struct {
	From   string `json:"from"`
	Reason string `json:"reason"`
}

type MoveResponse struct {
	Moved  []MoveCopyResult  `json:"moved"`
	Failed []MoveCopyFailure `json:"failed"`
}

type CopyResponse struct {
	Copied []MoveCopyResult  `json:"copied"`
	Failed []MoveCopyFailure `json:"failed"`
}

type DeleteRequest struct {
	Paths []string `json:"paths"`
}

type DeleteFailure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type DeleteResponse struct {
	Deleted []string        `json:"deleted"`
	Failed  []DeleteFailure `json:"failed"`
}

type RestoreRequest struct {
	Paths []string `json:"paths"`
}

type RestoreFailure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type RestoreResponse struct {
	Restored []string         `json:"restored"`
	Failed   []RestoreFailure `json:"failed"`
}

type AuditActor struct {
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
	IP       string `json:"ip,omitempty"`
}

type AuditEntry struct {
	Action     string     `json:"action"`
	OccurredAt string     `json:"occurred_at"`
	Actor      AuditActor `json:"actor"`
	Status     string     `json:"status"`
	Resource   string     `json:"resource,omitempty"`
	Before     any        `json:"before,omitempty"`
	After      any        `json:"after,omitempty"`
	Error      string     `json:"error,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type JobOperationRequest struct {
	Operation      string   `json:"operation"`
	Sources        []string `json:"sources,omitempty"`
	Destination    string   `json:"destination,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	ConflictPolicy string   `json:"conflict_policy,omitempty"`
}

type JobItemResult struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Path   string `json:"path,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type JobData struct {
	JobID          string          `json:"job_id"`
	Operation      string          `json:"operation"`
	Status         string          `json:"status"`
	ConflictPolicy string          `json:"conflict_policy,omitempty"`
	TotalItems     int             `json:"total_items"`
	ProcessedItems int             `json:"processed_items"`
	SuccessItems   int             `json:"success_items"`
	FailedItems    int             `json:"failed_items"`
	Progress       int             `json:"progress"`
	CreatedAt      string          `json:"created_at"`
	StartedAt      string          `json:"started_at,omitempty"`
	FinishedAt     string          `json:"finished_at,omitempty"`
	Items          []JobItemResult `json:"items,omitempty"`
}

type JobItemsData struct {
	JobID string          `json:"job_id"`
	Items []JobItemResult `json:"items"`
}
