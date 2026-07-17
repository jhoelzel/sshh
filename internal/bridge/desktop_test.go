package bridge

import (
	"strings"
	"testing"
	"time"

	sessionusecase "shh-h/internal/usecase/session"
)

func TestAttachFrontendIsIdempotentForSameInstance(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil)

	first, err := desktop.AttachFrontend("frontend-instance")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}
	second, err := desktop.AttachFrontend("frontend-instance")
	if err != nil {
		t.Fatalf("reattach frontend: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("same frontend instance received a new lease: %q != %q", first.ID, second.ID)
	}
	if _, err := time.Parse(time.RFC3339Nano, second.ExpiresAt); err != nil {
		t.Fatalf("lease expiry is not RFC3339: %v", err)
	}
}

func TestAttachFrontendReplacesPreviousInstance(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil)

	first, err := desktop.AttachFrontend("first-instance")
	if err != nil {
		t.Fatalf("attach first frontend: %v", err)
	}
	second, err := desktop.AttachFrontend("second-instance")
	if err != nil {
		t.Fatalf("attach second frontend: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("replacement frontend reused the previous lease")
	}
	if _, err := desktop.RenewFrontendLease(first.ID); err == nil {
		t.Fatal("expected the replaced lease to be rejected")
	}
	if _, err := desktop.RenewFrontendLease(second.ID); err != nil {
		t.Fatalf("renew active lease: %v", err)
	}
}

func TestAttachFrontendRejectsInvalidNonce(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil)

	if _, err := desktop.AttachFrontend("  "); err == nil {
		t.Fatal("expected an empty frontend nonce to be rejected")
	}
}

func TestTerminalTextFilenameSanitizesUntrustedTitles(t *testing.T) {
	filename := terminalTextFilename(" ../../Production\nShell ")
	if filename != "Production-Shell-selection.txt" {
		t.Fatalf("unexpected filename %q", filename)
	}
	if fallback := terminalTextFilename("///"); fallback != "terminal-selection.txt" {
		t.Fatalf("unexpected fallback filename %q", fallback)
	}
	if long := terminalTextFilename(strings.Repeat("界", 100)); len(long) > 100 {
		t.Fatalf("suggested filename exceeds byte budget: %d", len(long))
	}
}
