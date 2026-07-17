//go:build darwin || linux

package localpty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"

	"shh-h/internal/port"
)

type transport struct {
	command   *exec.Cmd
	pty       *os.File
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func (f *Factory) Open(ctx context.Context, spec port.TerminalSpec) (port.TerminalTransport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	shell, err := resolveShell(spec.Command)
	if err != nil {
		return nil, err
	}
	arguments := append([]string(nil), spec.Arguments...)
	if len(arguments) == 0 {
		arguments = []string{"-l"}
	}

	command := exec.Command(shell, arguments...)
	command.Dir = spec.WorkingDirectory
	if command.Dir == "" {
		command.Dir, _ = os.UserHomeDir()
	}
	command.Env = mergeEnvironment(os.Environ(), append(spec.Environment,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	))

	file, err := pty.StartWithSize(command, &pty.Winsize{Cols: spec.Columns, Rows: spec.Rows})
	if err != nil {
		return nil, fmt.Errorf("start local shell: %w", err)
	}

	return &transport{command: command, pty: file}, nil
}

func (t *transport) Read(buffer []byte) (int, error) {
	n, err := t.pty.Read(buffer)
	if errors.Is(err, syscall.EIO) {
		return n, io.EOF
	}
	return n, err
}

func (t *transport) Write(data []byte) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.pty.Write(data)
}

func (t *transport) Resize(ctx context.Context, columns, rows uint16) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := pty.Setsize(t.pty, &pty.Winsize{Cols: columns, Rows: rows}); err != nil {
		return fmt.Errorf("resize local shell: %w", err)
	}
	return nil
}

func (t *transport) Signal(ctx context.Context, signal port.TerminalSignal) error {
	if err := ctx.Err(); err != nil && signal != port.SignalKill {
		return err
	}
	if t.command.Process == nil {
		return nil
	}

	var native syscall.Signal
	switch signal {
	case port.SignalHangup:
		native = syscall.SIGHUP
	case port.SignalInterrupt:
		native = syscall.SIGINT
	case port.SignalTerminate:
		native = syscall.SIGTERM
	case port.SignalKill:
		native = syscall.SIGKILL
	default:
		return fmt.Errorf("unsupported terminal signal %q", signal)
	}

	err := syscall.Kill(-t.command.Process.Pid, native)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("signal local shell: %w", err)
	}
	return nil
}

func (t *transport) Wait() (port.ExitStatus, error) {
	err := t.command.Wait()
	if err == nil {
		return port.ExitStatus{Code: 0}, nil
	}

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return port.ExitStatus{}, fmt.Errorf("wait for local shell: %w", err)
	}

	status := port.ExitStatus{Code: exitError.ExitCode()}
	if waitStatus, ok := exitError.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
		status.Signal = waitStatus.Signal().String()
	}
	return status, nil
}

func (t *transport) Close() error {
	t.closeOnce.Do(func() {
		t.closeErr = t.pty.Close()
	})
	return t.closeErr
}

func resolveShell(configured string) (string, error) {
	candidates := []string{strings.TrimSpace(configured), strings.TrimSpace(os.Getenv("SHELL")), "/bin/zsh", "/bin/bash", "/bin/sh"}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New("no executable login shell found")
}

func mergeEnvironment(base, overrides []string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, item := range append(append([]string(nil), base...), overrides...) {
		key, value, ok := strings.Cut(item, "=")
		if ok && key != "" {
			values[key] = value
		}
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
