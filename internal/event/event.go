package event

type Type string

const (
	TypeFileCreated      Type = "file.created"
	TypeFileUploaded     Type = "file.uploaded"
	TypeFileDeleted      Type = "file.deleted"
	TypeFileMoved        Type = "file.moved"
	TypeFileCopied       Type = "file.copied"
	TypeDirCreated       Type = "dir.created"
	TypeJobStarted       Type = "job.started"
	TypeJobProgress      Type = "job.progress"
	TypeJobCompleted     Type = "job.completed"
	TypeJobFailed        Type = "job.failed"
	TypeFileCompressed   Type = "file.compressed"
	TypeFileDecompressed Type = "file.decompressed"
)

type Event struct {
	ID        string      `json:"id"`
	Type      Type        `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp string      `json:"timestamp"`
	ActorID   string      `json:"actor_id,omitempty"` // Who triggered the event
}

type Bus interface {
	Publish(e Event)
	Subscribe() (<-chan Event, func()) // Returns channel and unsubscribe function
}
