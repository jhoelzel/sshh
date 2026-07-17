package textfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicRoundTripAndPermissions(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "selection.txt")
	data := []byte("first line\nsecond line")
	if err := WriteAtomic(filename, data); err != nil {
		t.Fatalf("write text: %v", err)
	}
	written, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read text: %v", err)
	}
	if !bytes.Equal(written, data) {
		t.Fatalf("unexpected contents: %q", written)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat text: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Fatalf("unexpected permissions: %o", info.Mode().Perm())
	}
}

func TestWriteAtomicRejectsOversizedText(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "selection.txt")
	if err := WriteAtomic(filename, make([]byte, MaxBytes+1)); err == nil {
		t.Fatal("expected oversized text to be rejected")
	}
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Fatalf("oversized export created a destination: %v", err)
	}
}

func TestWriteAtomicRejectsEmptyPath(t *testing.T) {
	if err := WriteAtomic("  ", []byte("text")); err == nil {
		t.Fatal("expected empty path to be rejected")
	}
}
