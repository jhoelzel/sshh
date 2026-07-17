package transferstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	filedomain "shh-h/internal/domain/filetransfer"
)

func TestStoreRoundTripUsesPrivateVersionedFile(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "state", filename)
	store := NewAt(storePath)
	if resumes, err := store.LoadResumes(); err != nil || len(resumes) != 0 {
		t.Fatalf("load empty resumes: resumes=%#v err=%v", resumes, err)
	}
	records := []filedomain.ResumeRecord{downloadRecord(t), uploadRecord(t)}
	if err := store.SaveResumes(records); err != nil {
		t.Fatalf("save resumes: %v", err)
	}
	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("inspect resume metadata: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Fatalf("resume metadata permissions=%#o", info.Mode().Perm())
	}
	directoryInfo, err := os.Stat(filepath.Dir(storePath))
	if err != nil {
		t.Fatalf("inspect resume metadata directory: %v", err)
	}
	if directoryInfo.Mode().Perm() != directoryMode {
		t.Fatalf("resume metadata directory permissions=%#o", directoryInfo.Mode().Perm())
	}

	reloaded := NewAt(storePath)
	actual, err := reloaded.LoadResumes()
	if err != nil {
		t.Fatalf("reload resumes: %v", err)
	}
	if len(actual) != 2 || actual[0].ID != records[0].ID || actual[1].ID != records[1].ID {
		t.Fatalf("unexpected resumes: %#v", actual)
	}
}

func TestStoreRejectsTamperedPartialPathAndUnknownFields(t *testing.T) {
	record := downloadRecord(t)
	record.PartialPath = filepath.Join(t.TempDir(), "unrelated")
	store := NewAt(filepath.Join(t.TempDir(), filename))
	if err := store.SaveResumes([]filedomain.ResumeRecord{record}); err == nil {
		t.Fatal("expected unrelated partial path to fail validation")
	}

	data := map[string]any{"version": currentSchema, "revision": 1, "resumes": []any{}, "extra": true}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("encode invalid document: %v", err)
	}
	storePath := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(storePath, encoded, fileMode); err != nil {
		t.Fatalf("write invalid document: %v", err)
	}
	if _, err := NewAt(storePath).LoadResumes(); err == nil {
		t.Fatal("expected unknown field to fail")
	}
}

func TestStoreRejectsNonCanonicalPaths(t *testing.T) {
	record := downloadRecord(t)
	record.Source = "/remote/../remote/download.bin"
	if err := NewAt(filepath.Join(t.TempDir(), filename)).SaveResumes([]filedomain.ResumeRecord{record}); err == nil {
		t.Fatal("expected non-canonical source path to fail validation")
	}

	record = uploadRecord(t)
	record.Destination = "/remote/./upload.bin"
	if err := NewAt(filepath.Join(t.TempDir(), filename)).SaveResumes([]filedomain.ResumeRecord{record}); err == nil {
		t.Fatal("expected non-canonical destination path to fail validation")
	}
}

func TestStoreDetectsExternalChanges(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), filename)
	store := NewAt(storePath)
	if _, err := store.LoadResumes(); err != nil {
		t.Fatalf("load resumes: %v", err)
	}
	if err := store.SaveResumes([]filedomain.ResumeRecord{downloadRecord(t)}); err != nil {
		t.Fatalf("save resumes: %v", err)
	}
	if err := os.WriteFile(storePath, []byte("{}\n"), fileMode); err != nil {
		t.Fatalf("replace resume metadata: %v", err)
	}
	if err := store.SaveResumes(nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected external-change conflict, got %v", err)
	}
}

func downloadRecord(t *testing.T) filedomain.ResumeRecord {
	t.Helper()
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	destination := filepath.Join(t.TempDir(), "download.bin")
	id := "00112233445566778899aabbccddeeff"
	return filedomain.ResumeRecord{
		ID: id, ProfileID: "profile-1", Direction: filedomain.DirectionDownload,
		Source: "/remote/download.bin", Destination: destination,
		PartialPath: localPartialPath(destination, id), Bytes: 4, Total: 8, SourceSize: 8,
		SourceModifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func uploadRecord(t *testing.T) filedomain.ResumeRecord {
	t.Helper()
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	id := "102132435465768798a9bacbdcedfe0f"
	destination := "/remote/upload.bin"
	return filedomain.ResumeRecord{
		ID: id, ProfileID: "profile-1", Direction: filedomain.DirectionUpload,
		Source: filepath.Join(t.TempDir(), "upload.bin"), Destination: destination,
		PartialPath: remotePartialPath(destination, id), Bytes: 2, Total: 8, SourceSize: 8,
		SourceModifiedAt: now,
		SourceSHA256:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CreatedAt:        now, UpdatedAt: now,
	}
}
