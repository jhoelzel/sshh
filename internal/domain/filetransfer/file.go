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

const (
	DirectionDownload Direction = "download"
	DirectionUpload   Direction = "upload"
)

type TransferState string

const (
	TransferQueued    TransferState = "queued"
	TransferRunning   TransferState = "running"
	TransferCompleted TransferState = "completed"
	TransferFailed    TransferState = "failed"
	TransferCancelled TransferState = "cancelled"
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
	StartedAt   string        `json:"startedAt"`
	FinishedAt  string        `json:"finishedAt"`
}
