package sessionlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shh-h/internal/port"
)

func TestFileLogWritesPrivatelyAndRotates(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs")
	factory := NewAt(directory)
	factory.now = func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }
	opened, err := factory.Open(port.SessionLogSpec{
		SessionID: "1234567890", Title: "Production shell", MaxBytes: 6, RotationFiles: 2,
	})
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	if _, err := opened.Write([]byte("first")); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if _, err := opened.Write([]byte("second")); err != nil {
		t.Fatalf("write second: %v", err)
	}
	path := opened.Path()
	if err := opened.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
	active, err := os.ReadFile(path)
	if err != nil || string(active) != "second" {
		t.Fatalf("read active log: data=%q err=%v", active, err)
	}
	rotated, err := os.ReadFile(path + ".1")
	if err != nil || string(rotated) != "first" {
		t.Fatalf("read rotated log: data=%q err=%v", rotated, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("active permissions: info=%v err=%v", info, err)
	}
}

func TestFileLogPrefixesLogicalLinesAcrossWrites(t *testing.T) {
	factory := NewAt(t.TempDir())
	instant := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	factory.now = func() time.Time { return instant }
	opened, err := factory.Open(port.SessionLogSpec{
		SessionID: "session", Title: "Shell", TimestampLines: true, MaxBytes: 1024, RotationFiles: 1,
	})
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	_, _ = opened.Write([]byte("hel"))
	_, _ = opened.Write([]byte("lo\nnext\n"))
	path := opened.Path()
	if err := opened.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	prefix := "[2026-07-17T12:00:00Z] "
	if string(data) != prefix+"hello\n"+prefix+"next\n" {
		t.Fatalf("unexpected timestamped log %q", data)
	}
	if strings.Count(string(data), prefix) != 2 {
		t.Fatalf("unexpected timestamp count in %q", data)
	}
}
