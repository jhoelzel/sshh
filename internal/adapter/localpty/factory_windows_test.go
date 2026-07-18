//go:build windows

package localpty

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"shh-h/internal/port"
)

const (
	conPTYHelperEnvironment = "SHHH_CONPTY_TEST_HELPER"
	conPTYChildEnvironment  = "SHHH_CONPTY_TEST_CHILD"
)

var childProcessPattern = regexp.MustCompile(`child:(\d+)`)

func TestMergeWindowsEnvironmentUsesCaseInsensitiveLastValue(t *testing.T) {
	base := []string{"Path=C:\\base", "KEEP=one", "=C:=C:\\work", "invalid"}
	overrides := []string{"PATH=C:\\override", "EMPTY="}
	got := mergeWindowsEnvironment(base, overrides)
	want := []string{"=C:=C:\\work", "EMPTY=", "KEEP=one", "PATH=C:\\override"}
	if !slices.Equal(got, want) {
		t.Fatalf("merged environment = %#v, want %#v", got, want)
	}
	if base[0] != "Path=C:\\base" || overrides[0] != "PATH=C:\\override" {
		t.Fatal("environment merge mutated an input slice")
	}
}

func TestCreateWindowsEnvironmentBlockIsUnicodeAndDoubleNullTerminated(t *testing.T) {
	block, err := createWindowsEnvironmentBlock([]string{"ALPHA=one", "UNICODE=Grüße"})
	if err != nil {
		t.Fatalf("create environment block: %v", err)
	}
	if len(block) < 2 || block[len(block)-1] != 0 || block[len(block)-2] != 0 {
		t.Fatalf("environment block is not double-null terminated: %#v", block)
	}
	decoded := string(utf16.Decode(block))
	if !strings.Contains(decoded, "ALPHA=one\x00") || !strings.Contains(decoded, "UNICODE=Grüße\x00") {
		t.Fatalf("environment block lost values: %q", decoded)
	}
	if _, err := createWindowsEnvironmentBlock([]string{"VALID=before\x00after"}); err == nil {
		t.Fatal("environment block accepted a null byte")
	}
	empty, err := createWindowsEnvironmentBlock(nil)
	if err != nil || !slices.Equal(empty, []uint16{0, 0}) {
		t.Fatalf("empty environment block = %#v, %v", empty, err)
	}
}

func TestResolveWindowsShellFindsSupportedDefault(t *testing.T) {
	shell, err := resolveWindowsShell("")
	if err != nil {
		t.Fatalf("resolve default Windows shell: %v", err)
	}
	switch strings.ToLower(filepath.Base(shell)) {
	case "pwsh.exe", "powershell.exe", "cmd.exe", "wsl.exe":
	default:
		t.Fatalf("resolved unsupported default Windows shell %q", shell)
	}
}

func TestFactoryRunsAndResizesRealConPTY(t *testing.T) {
	terminal, output := openConPTYFixture(t, 91, 33)
	waitForOutput(t, output, "ready:exact value:term=xterm-256color:color=truecolor:size=91x33", 10*time.Second)

	if err := terminal.Resize(context.Background(), 101, 37); err != nil {
		t.Fatalf("resize ConPTY: %v", err)
	}
	const resizedReport = "report:exact value:term=xterm-256color:color=truecolor:size=101x37"
	for attempt := 0; attempt < 20 && !output.contains(resizedReport); attempt++ {
		if _, err := terminal.Write([]byte("report\r\n")); err != nil {
			t.Fatalf("request resized dimensions: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	waitForOutput(t, output, resizedReport, 2*time.Second)

	if _, err := terminal.Write([]byte("exit23\r\n")); err != nil {
		t.Fatalf("request fixture exit: %v", err)
	}
	status, err := waitForTransport(t, terminal, 10*time.Second)
	if err != nil {
		t.Fatalf("wait for ConPTY fixture: %v", err)
	}
	if status.Code != 23 || status.Signal != "" {
		t.Fatalf("ConPTY exit status = %#v, want code 23", status)
	}
}

func TestFactoryTerminatesConPTYDescendantProcessTree(t *testing.T) {
	terminal, output := openConPTYFixture(t, 80, 24)
	waitForOutput(t, output, "ready:", 10*time.Second)
	if _, err := terminal.Write([]byte("spawn\r\n")); err != nil {
		t.Fatalf("spawn ConPTY descendant: %v", err)
	}
	waitForOutput(t, output, "child:", 10*time.Second)
	match := childProcessPattern.FindStringSubmatch(output.text())
	if len(match) != 2 {
		t.Fatalf("ConPTY output did not contain a child PID: %q", output.text())
	}
	childID, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil {
		t.Fatalf("parse child PID %q: %v", match[1], err)
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(childID))
	if err != nil {
		t.Fatalf("open ConPTY descendant %d: %v", childID, err)
	}
	defer windows.CloseHandle(child)

	if err := terminal.Signal(context.Background(), port.SignalTerminate); err != nil {
		t.Fatalf("terminate ConPTY process tree: %v", err)
	}
	status, err := waitForTransport(t, terminal, 10*time.Second)
	if err != nil {
		t.Fatalf("wait for terminated ConPTY fixture: %v", err)
	}
	if status.Code != terminateExitCode || status.Signal != string(port.SignalTerminate) {
		t.Fatalf("terminated ConPTY status = %#v", status)
	}
	event, err := windows.WaitForSingleObject(child, 5000)
	if err != nil {
		t.Fatalf("wait for ConPTY descendant: %v", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		t.Fatalf("ConPTY descendant %d remained alive; wait event %#x", childID, event)
	}
}

func TestFactoryClosesConPTYWhileOutputIsActive(t *testing.T) {
	terminal, output := openConPTYFixture(t, 80, 24)
	waitForOutput(t, output, "ready:", 10*time.Second)
	if _, err := terminal.Write([]byte("flood\r\n")); err != nil {
		t.Fatalf("start ConPTY output flood: %v", err)
	}
	waitForOutput(t, output, "flood:start", 10*time.Second)
	if err := terminal.Signal(context.Background(), port.SignalHangup); err != nil {
		t.Fatalf("hang up ConPTY fixture: %v", err)
	}
	if err := terminal.Close(); err != nil {
		t.Fatalf("close ConPTY during output: %v", err)
	}

	waitDone := make(chan struct{})
	var status port.ExitStatus
	var waitErr error
	go func() {
		status, waitErr = terminal.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		if err := terminal.Signal(context.Background(), port.SignalTerminate); err != nil {
			t.Fatalf("escalate ConPTY close: %v", err)
		}
		select {
		case <-waitDone:
		case <-time.After(8 * time.Second):
			t.Fatal("ConPTY close during output did not reap the process")
		}
	}
	if waitErr != nil {
		t.Fatalf("wait after ConPTY close: %v", waitErr)
	}
	if status.Signal != "" && status.Signal != string(port.SignalTerminate) {
		t.Fatalf("unexpected ConPTY close status: %#v", status)
	}
}

func TestFactoryWaitDoesNotDeadlockWithoutOutputReader(t *testing.T) {
	terminal := newConPTYFixture(t, 80, 24)
	t.Cleanup(func() {
		_ = terminal.Signal(context.Background(), port.SignalKill)
		_ = terminal.Close()
		_, _ = terminal.Wait()
	})
	if _, err := terminal.Write([]byte("flood\r\n")); err != nil {
		t.Fatalf("start unread ConPTY output flood: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if err := terminal.Signal(context.Background(), port.SignalKill); err != nil {
		t.Fatalf("kill ConPTY with unread output: %v", err)
	}

	status, err := waitForTransport(t, terminal, conPTYCloseDrainTimeout+5*time.Second)
	if err != nil {
		t.Fatalf("wait for ConPTY with unread output: %v", err)
	}
	if status.Code != killExitCode || status.Signal != string(port.SignalKill) {
		t.Fatalf("unread-output ConPTY status = %#v", status)
	}
}

func TestConPTYHelperProcess(t *testing.T) {
	if os.Getenv(conPTYHelperEnvironment) != "1" {
		return
	}
	os.Exit(runConPTYHelper())
}

func TestConPTYChildProcess(t *testing.T) {
	if os.Getenv(conPTYChildEnvironment) != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func runConPTYHelper() int {
	reportConPTYFixture("ready")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "report":
			reportConPTYFixture("report")
		case "exit23":
			return 23
		case "spawn":
			command := exec.Command(os.Args[0], "-test.run=^TestConPTYChildProcess$")
			command.Env = append(os.Environ(), conPTYChildEnvironment+"=1")
			if err := command.Start(); err != nil {
				fmt.Printf("spawn-error:%v\r\n", err)
				continue
			}
			fmt.Printf("child:%d\r\n", command.Process.Pid)
			_ = command.Process.Release()
		case "flood":
			fmt.Print("flood:start\r\n")
			chunk := bytes.Repeat([]byte("x"), 4096)
			for index := 0; index < 512; index++ {
				_, _ = os.Stdout.Write(chunk)
			}
			fmt.Print("\r\nflood:end\r\n")
		}
	}
	return 0
}

func reportConPTYFixture(label string) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Stdout, &info); err != nil {
		fmt.Printf("%s:size-error:%v\r\n", label, err)
		return
	}
	fmt.Printf(
		"%s:%s:term=%s:color=%s:size=%dx%d\r\n",
		label,
		os.Getenv("SHHH_PROFILE_TEST"),
		os.Getenv("TERM"),
		os.Getenv("COLORTERM"),
		info.Size.X,
		info.Size.Y,
	)
}

func openConPTYFixture(t *testing.T, columns, rows uint16) (port.TerminalTransport, *lockedOutput) {
	t.Helper()
	terminal := newConPTYFixture(t, columns, rows)
	output := &lockedOutput{done: make(chan struct{})}
	go func() {
		_, _ = io.Copy(output, terminal)
		close(output.done)
	}()
	t.Cleanup(func() {
		_ = terminal.Signal(context.Background(), port.SignalKill)
		_ = terminal.Close()
		_, _ = terminal.Wait()
		select {
		case <-output.done:
		case <-time.After(5 * time.Second):
			t.Error("ConPTY output reader did not stop")
		}
	})
	return terminal, output
}

func newConPTYFixture(t *testing.T, columns, rows uint16) port.TerminalTransport {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find ConPTY fixture executable: %v", err)
	}
	terminal, err := NewFactory().Open(context.Background(), port.TerminalSpec{
		Command:   executable,
		Arguments: []string{"-test.run=^TestConPTYHelperProcess$"},
		Environment: []string{
			conPTYHelperEnvironment + "=1",
			"SHHH_PROFILE_TEST=exact value",
		},
		Columns: columns,
		Rows:    rows,
	})
	if err != nil {
		t.Fatalf("open ConPTY fixture: %v", err)
	}
	return terminal
}

func waitForOutput(t *testing.T, output *lockedOutput, value string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if output.contains(value) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in ConPTY output %q", value, output.text())
}

func waitForTransport(t *testing.T, terminal port.TerminalTransport, timeout time.Duration) (port.ExitStatus, error) {
	t.Helper()
	type result struct {
		status port.ExitStatus
		err    error
	}
	done := make(chan result, 1)
	go func() {
		status, err := terminal.Wait()
		done <- result{status: status, err: err}
	}()
	select {
	case result := <-done:
		return result.status, result.err
	case <-time.After(timeout):
		t.Fatal("timed out waiting for ConPTY process")
		return port.ExitStatus{}, context.DeadlineExceeded
	}
}

type lockedOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	done   chan struct{}
}

func (o *lockedOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.Write(data)
}

func (o *lockedOutput) contains(value string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Contains(o.buffer.Bytes(), []byte(value))
}

func (o *lockedOutput) text() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}
