//go:build darwin || linux

package localpty

import (
	"bytes"
	"context"
	"errors"
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

func TestFactoryPreservesBinaryInputAndAppliesLiveResize(t *testing.T) {
	transport, err := NewFactory().Open(context.Background(), port.TerminalSpec{
		Command:   "/bin/sh",
		Arguments: []string{"-c", "stty raw -echo; printf 'ready\\n'; dd bs=1 count=6 2>/dev/null | od -An -tx1; stty size"},
		Columns:   80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("open PTY: %v", err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			_ = transport.Signal(context.Background(), port.SignalKill)
			_, _ = transport.Wait()
		}
		_ = transport.Close()
	})

	type readResult struct {
		output []byte
		err    error
	}
	ready := make(chan struct{}, 1)
	readDone := make(chan readResult, 1)
	go func() {
		output := make([]byte, 0, 128)
		buffer := make([]byte, 64)
		for {
			n, readErr := transport.Read(buffer)
			output = append(output, buffer[:n]...)
			if bytes.Contains(output, []byte("ready\n")) {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
			if readErr != nil {
				readDone <- readResult{output: output, err: readErr}
				return
			}
		}
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PTY input fixture")
	}
	if err := transport.Resize(context.Background(), 120, 40); err != nil {
		t.Fatalf("resize PTY: %v", err)
	}
	payload := []byte{'A', 0, 0xff, 0x1b, '[', 'M'}
	if written, err := transport.Write(payload); err != nil || written != len(payload) {
		t.Fatalf("write PTY input: wrote %d bytes: %v", written, err)
	}

	status, err := transport.Wait()
	reaped = true
	if err != nil {
		t.Fatalf("wait for PTY input fixture: %v", err)
	}
	if status.Code != 0 {
		t.Fatalf("PTY input fixture exit code = %d", status.Code)
	}

	select {
	case result := <-readDone:
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			t.Fatalf("read PTY output: %v", result.err)
		}
		text := strings.Join(strings.Fields(strings.ToLower(string(result.output))), " ")
		if !strings.Contains(text, "41 00 ff 1b 5b 4d") {
			t.Fatalf("binary input did not reach PTY unchanged: %q", text)
		}
		if !strings.Contains(text, "40 120") {
			t.Fatalf("live resize did not reach PTY: %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out reading PTY input fixture output")
	}
}
