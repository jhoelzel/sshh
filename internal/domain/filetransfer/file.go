package filetransfer

type Entry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Directory  bool   `json:"directory"`
	Symlink    bool   `json:"symlink"`
	Size       int64  `json:"size"`
	Mode       uint32 `json:"mode"`
	ModifiedAt string `json:"modifiedAt"`
}

type Session struct {
	ID        string `json:"id"`
	LeaseID   string `json:"leaseId"`
	ProfileID string `json:"profileId"`
	Root      string `json:"root"`
	OpenedAt  string `json:"openedAt"`
}

type Direction string
type CollisionPolicy string

const (
	DirectionDownload Direction = "download"
	DirectionUpload   Direction = "upload"

	CollisionAsk       CollisionPolicy = "ask"
	CollisionOverwrite CollisionPolicy = "overwrite"
	CollisionSkip      CollisionPolicy = "skip"
	CollisionRename    CollisionPolicy = "rename"

	MinConcurrency = 1
	MaxConcurrency = 8
)

type TransferState string

const (
	TransferQueued    TransferState = "queued"
	TransferRunning   TransferState = "running"
	TransferCompleted TransferState = "completed"
	TransferFailed    TransferState = "failed"
	TransferCancelled TransferState = "cancelled"
	TransferSkipped   TransferState = "skipped"
)

type Transfer struct {
	ID          string        `json:"id"`
	LeaseID     string        `json:"leaseId"`
	SessionID   string        `json:"sessionId"`
	Direction   Direction     `json:"direction"`
	Source      string        `json:"source"`
	Destination string        `json:"destination"`
	Bytes       int64         `json:"bytes"`
	Total       int64         `json:"total"`
	State       TransferState `json:"state"`
	Message     string        `json:"message"`
	ResumeID    string        `json:"resumeId"`
	ResumedFrom int64         `json:"resumedFrom"`
	StartedAt   string        `json:"startedAt"`
	FinishedAt  string        `json:"finishedAt"`
}

type ResumeRecord struct {
	ID               string    `json:"id"`
	ProfileID        string    `json:"profileId"`
	Direction        Direction `json:"direction"`
	Source           string    `json:"source"`
	Destination      string    `json:"destination"`
	PartialPath      string    `json:"partialPath"`
	Bytes            int64     `json:"bytes"`
	Total            int64     `json:"total"`
	SourceSize       int64     `json:"sourceSize"`
	SourceModifiedAt string    `json:"sourceModifiedAt"`
	SourceSHA256     string    `json:"sourceSha256,omitempty"`
	Overwrite        bool      `json:"overwrite"`
	LastError        string    `json:"lastError,omitempty"`
	CreatedAt        string    `json:"createdAt"`
	UpdatedAt        string    `json:"updatedAt"`
}

type TransferResume struct {
	ID          string    `json:"id"`
	ProfileID   string    `json:"profileId"`
	Direction   Direction `json:"direction"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Bytes       int64     `json:"bytes"`
	Total       int64     `json:"total"`
	Available   bool      `json:"available"`
	Message     string    `json:"message"`
	CreatedAt   string    `json:"createdAt"`
	UpdatedAt   string    `json:"updatedAt"`
}
