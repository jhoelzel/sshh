package port

type SessionLogSpec struct {
	SessionID      string
	Title          string
	TimestampLines bool
	MaxBytes       int64
	RotationFiles  int
}

type SessionLog interface {
	Write([]byte) (int, error)
	Path() string
	BytesWritten() int64
	Close() error
}

type SessionLogFactory interface {
	Open(SessionLogSpec) (SessionLog, error)
}
