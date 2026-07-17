//go:build darwin || linux

package localpty

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"shh-h/internal/port"
)

func TestFactoryRunsCommandInRealPTY(t *testing.T) {
	transport, err := NewFactory().Open(context.Background(), port.TerminalSpec{
		Command: "/bin/sh", Arguments: []string{"-c", "printf 'ready\\n'; stty size"},
		Columns: 91, Rows: 33,
	})
	if err != nil {
		t.Fatalf("open PTY: %v", err)
	}
	defer transport.Close()

	readDone := make(chan []byte, 1)
	go func() {
		output, _ := io.ReadAll(transport)
		readDone <- output
	}()

	status, err := transport.Wait()
	if err != nil {
		t.Fatalf("wait for command: %v", err)
	}
	if status.Code != 0 {
		t.Fatalf("expected exit code 0, got %d", status.Code)
	}

	select {
	case output := <-readDone:
		text := strings.ReplaceAll(string(output), "\r", "")
		if !strings.Contains(text, "ready\n") || !strings.Contains(text, "33 91") {
			t.Fatalf("unexpected PTY output %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out reading PTY output")
	}
}
