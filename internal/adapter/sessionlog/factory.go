package sessionlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"shh-h/internal/port"
)

const (
	directoryMode = 0o700
	fileMode      = 0o600
	maxTitleRunes = 48
)

type Factory struct {
	directory string
	now       func() time.Time
}

func New(appID string) (*Factory, error) {
	if strings.TrimSpace(appID) == "" {
		return nil, errors.New("app id is required")
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config directory: %w", err)
	}
	return NewAt(filepath.Join(directory, appID, "logs")), nil
}

func NewAt(directory string) *Factory {
	return &Factory{directory: directory, now: time.Now}
}

func (f *Factory) Open(spec port.SessionLogSpec) (port.SessionLog, error) {
	if strings.TrimSpace(spec.SessionID) == "" {
		return nil, errors.New("session id is required")
	}
	if spec.MaxBytes <= 0 {
		return nil, errors.New("positive log rotation size is required")
	}
	if spec.RotationFiles < 0 || spec.RotationFiles > 20 {
		return nil, errors.New("log rotation file count must be between 0 and 20")
	}
	if err := os.MkdirAll(f.directory, directoryMode); err != nil {
		return nil, fmt.Errorf("create session log directory: %w", err)
	}
	if err := os.Chmod(f.directory, directoryMode); err != nil {
		return nil, fmt.Errorf("protect session log directory: %w", err)
	}
	now := f.now().UTC()
	shortID := spec.SessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	baseName := fmt.Sprintf("%s-%s-%s", sanitizeTitle(spec.Title), now.Format("20060102-150405.000"), shortID)
	file, path, err := openUnique(f.directory, baseName)
	if err != nil {
		return nil, fmt.Errorf("create session log: %w", err)
	}
	if err := file.Chmod(fileMode); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("protect session log: %w", err)
	}
	return &fileLog{
		file: file, path: path, timestampLines: spec.TimestampLines, atLineStart: true,
		maxBytes: spec.MaxBytes, rotationFiles: spec.RotationFiles, now: f.now,
	}, nil
}

type fileLog struct {
	mu             sync.Mutex
	file           *os.File
	path           string
	timestampLines bool
	atLineStart    bool
	maxBytes       int64
	rotationFiles  int
	currentBytes   int64
	totalBytes     int64
	closed         bool
	now            func() time.Time
}

func (l *fileLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	if len(data) == 0 {
		return 0, nil
	}
	encoded := data
	if l.timestampLines {
		encoded = l.withTimestamps(data)
	}
	if l.currentBytes > 0 && l.currentBytes+int64(len(encoded)) > l.maxBytes {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	if err := writeAll(l.file, encoded); err != nil {
		return 0, fmt.Errorf("write session log: %w", err)
	}
	l.currentBytes += int64(len(encoded))
	l.totalBytes += int64(len(encoded))
	return len(data), nil
}

func (l *fileLog) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

func (l *fileLog) BytesWritten() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.totalBytes
}

func (l *fileLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if err := l.file.Sync(); err != nil {
		_ = l.file.Close()
		return fmt.Errorf("sync session log: %w", err)
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close session log: %w", err)
	}
	return nil
}

func (l *fileLog) withTimestamps(data []byte) []byte {
	result := make([]byte, 0, len(data)+64)
	for _, value := range data {
		if l.atLineStart {
			result = append(result, '[')
			result = l.now().UTC().AppendFormat(result, time.RFC3339Nano)
			result = append(result, ']', ' ')
			l.atLineStart = false
		}
		result = append(result, value)
		if value == '\n' {
			l.atLineStart = true
		}
	}
	return result
}

func (l *fileLog) rotate() error {
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("sync session log before rotation: %w", err)
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close session log before rotation: %w", err)
	}
	if l.rotationFiles > 0 {
		_ = os.Remove(fmt.Sprintf("%s.%d", l.path, l.rotationFiles))
		for index := l.rotationFiles - 1; index >= 1; index-- {
			from := fmt.Sprintf("%s.%d", l.path, index)
			to := fmt.Sprintf("%s.%d", l.path, index+1)
			if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("rotate session log: %w", err)
			}
		}
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return fmt.Errorf("rotate active session log: %w", err)
		}
	}
	file, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("create rotated session log: %w", err)
	}
	l.file = file
	l.currentBytes = 0
	return nil
}

func openUnique(directory, baseName string) (*os.File, string, error) {
	for suffix := 1; suffix <= 100; suffix++ {
		name := baseName + ".log"
		if suffix > 1 {
			name = fmt.Sprintf("%s-%d.log", baseName, suffix)
		}
		path := filepath.Join(directory, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, path, err
	}
	return nil, "", errors.New("too many session logs share the same name")
}

func sanitizeTitle(title string) string {
	title = strings.TrimSpace(title)
	var result strings.Builder
	lastSeparator := false
	count := 0
	for _, character := range title {
		if count >= maxTitleRunes {
			break
		}
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '.' || character == '_' || character == '-' {
			result.WriteRune(character)
			lastSeparator = false
			count++
			continue
		}
		if !lastSeparator && result.Len() > 0 {
			result.WriteByte('-')
			lastSeparator = true
			count++
		}
	}
	value := strings.Trim(result.String(), "-.")
	if value == "" {
		return "session"
	}
	return value
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

var _ port.SessionLogFactory = (*Factory)(nil)
var _ port.SessionLog = (*fileLog)(nil)
