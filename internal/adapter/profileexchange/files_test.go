package profileexchange

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadFileAtomicallyWithPrivatePermissions(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "profiles.json")
	data := []byte("portable profiles\n")
	if err := WriteFile(filename, data); err != nil {
		t.Fatalf("write: %v", err)
	}
	read, err := ReadFile(filename)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(read, data) {
		t.Fatalf("unexpected contents: %q", read)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != portableFileMode {
		t.Fatalf("unexpected permissions: %o", info.Mode().Perm())
	}
}

func TestReadFileRejectsOversizedSources(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "too-large")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := file.Truncate(MaxFileSize + 1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := ReadFile(filename); err == nil {
		t.Fatal("expected oversized source to fail")
	}
}
