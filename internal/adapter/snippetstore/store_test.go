package snippetstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"shh-h/internal/domain/snippet"
)

func TestStoreRoundTripPermissionsAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "snippets.json")
	store := NewAt(path)
	loaded, err := store.LoadSnippets()
	if err != nil || len(loaded) != 0 {
		t.Fatalf("load empty store: snippets=%#v err=%v", loaded, err)
	}
	items := []snippet.Snippet{{ID: "deploy", Name: "Deploy", Body: "deploy {{target}}"}}
	if err := store.SaveSnippets(items); err != nil {
		t.Fatalf("save snippets: %v", err)
	}
	reloaded, err := NewAt(path).LoadSnippets()
	if err != nil || len(reloaded) != 1 || reloaded[0].Name != "Deploy" {
		t.Fatalf("reload snippets: snippets=%#v err=%v", reloaded, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat snippets: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected snippets mode %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snippets: %v", err)
	}
	if err := os.WriteFile(path, append(data, ' '), 0o600); err != nil {
		t.Fatalf("change snippets externally: %v", err)
	}
	if err := store.SaveSnippets(items); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestStoreRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snippets.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"revision":1,"snippets":[],"extra":true}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := NewAt(path).LoadSnippets(); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}
