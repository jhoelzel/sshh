//go:build windows

package localpty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"shh-h/internal/port"
)

const (
	terminateExitCode       = 1
	killExitCode            = 137
	conPTYCloseDrainTimeout = 2 * time.Second
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	updateProcThreadAttribute = kernel32.NewProc("UpdateProcThreadAttribute")
	requiredConPTYProcedures  = []struct {
		name      string
		procedure *windows.LazyProc
	}{
		{name: "CreatePseudoConsole", procedure: kernel32.NewProc("CreatePseudoConsole")},
		{name: "ResizePseudoConsole", procedure: kernel32.NewProc("ResizePseudoConsole")},
		{name: "ClosePseudoConsole", procedure: kernel32.NewProc("ClosePseudoConsole")},
	}
)

type transport struct {
	input   *os.File
	output  *os.File
	process windows.Handle
	job     windows.Handle
	console windows.Handle

	writeMu sync.Mutex
	stateMu sync.Mutex

	closeOnce sync.Once
	closeErr  error
	waitOnce  sync.Once
	waitState port.ExitStatus
	waitErr   error

	terminationSignal port.TerminalSignal
}

func (f *Factory) Open(ctx context.Context, spec port.TerminalSpec) (_ port.TerminalTransport, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ensureConPTYAvailable(); err != nil {
		return nil, err
	}
	size, err := conPTYSize(spec.Columns, spec.Rows)
	if err != nil {
		return nil, err
	}

	command, err := resolveWindowsShell(spec.Command)
	if err != nil {
		return nil, err
	}
	workingDirectory, err := resolveWindowsWorkingDirectory(spec.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	environment := mergeWindowsEnvironment(os.Environ(), append(
		append([]string(nil), spec.Environment...),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	))
	environmentBlock, err := createWindowsEnvironmentBlock(environment)
	if err != nil {
		return nil, fmt.Errorf("prepare local shell environment: %w", err)
	}
	application, commandLine, directory, err := windowsProcessParameters(command, spec.Arguments, workingDirectory)
	if err != nil {
		return nil, err
	}

	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	var console, job windows.Handle
	var processInfo windows.ProcessInformation
	defer func() {
		if resultErr == nil {
			return
		}
		closeHandle(&inputWrite)
		closeHandle(&outputRead)
		if processInfo.Process != 0 {
			_ = windows.TerminateProcess(processInfo.Process, killExitCode)
			_, _ = windows.WaitForSingleObject(processInfo.Process, windows.INFINITE)
		}
		closeHandle(&processInfo.Thread)
		closeHandle(&processInfo.Process)
		closeHandle(&job)
		if console != 0 {
			windows.ClosePseudoConsole(console)
			console = 0
		}
		closeHandle(&inputRead)
		closeHandle(&outputWrite)
	}()

	if err := windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	if err := windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY output pipe: %w", err)
	}
	if err := windows.SetHandleInformation(inputWrite, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, fmt.Errorf("protect ConPTY input handle: %w", err)
	}
	if err := windows.SetHandleInformation(outputRead, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, fmt.Errorf("protect ConPTY output handle: %w", err)
	}
	if err := windows.CreatePseudoConsole(size, inputRead, outputWrite, 0, &console); err != nil {
		return nil, fmt.Errorf("create ConPTY: %w", err)
	}

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("allocate ConPTY process attributes: %w", err)
	}
	defer attributes.Delete()
	if err := attachPseudoConsole(attributes, console); err != nil {
		return nil, fmt.Errorf("attach ConPTY process attribute: %w", err)
	}

	job, err = newTerminalJob()
	if err != nil {
		return nil, err
	}
	startupInfo := windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))},
		ProcThreadAttributeList: attributes.List(),
	}
	creationFlags := uint32(
		windows.EXTENDED_STARTUPINFO_PRESENT |
			windows.CREATE_UNICODE_ENVIRONMENT |
			windows.CREATE_SUSPENDED |
			windows.CREATE_DEFAULT_ERROR_MODE,
	)
	if err := windows.CreateProcess(
		application,
		&commandLine[0],
		nil,
		nil,
		false,
		creationFlags,
		&environmentBlock[0],
		directory,
		&startupInfo.StartupInfo,
		&processInfo,
	); err != nil {
		return nil, fmt.Errorf("start local shell in ConPTY: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, processInfo.Process); err != nil {
		return nil, fmt.Errorf("assign local shell to cleanup job: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := windows.ResumeThread(processInfo.Thread); err != nil {
		return nil, fmt.Errorf("resume local shell: %w", err)
	}

	closeHandle(&processInfo.Thread)
	closeHandle(&inputRead)
	closeHandle(&outputWrite)
	result := &transport{
		input:   os.NewFile(uintptr(inputWrite), "ConPTY input"),
		output:  os.NewFile(uintptr(outputRead), "ConPTY output"),
		process: processInfo.Process,
		job:     job,
		console: console,
	}
	inputWrite = 0
	outputRead = 0
	processInfo.Process = 0
	job = 0
	console = 0
	return result, nil
}

func (t *transport) Read(buffer []byte) (int, error) {
	t.stateMu.Lock()
	output := t.output
	t.stateMu.Unlock()
	if output == nil {
		return 0, io.EOF
	}
	n, err := output.Read(buffer)
	if errors.Is(err, os.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, windows.ERROR_BROKEN_PIPE) ||
		errors.Is(err, windows.ERROR_INVALID_HANDLE) ||
		errors.Is(err, windows.ERROR_NO_DATA) ||
		errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		return n, io.EOF
	}
	return n, err
}

func (t *transport) Write(data []byte) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	t.stateMu.Lock()
	input := t.input
	t.stateMu.Unlock()
	if input == nil {
		return 0, io.ErrClosedPipe
	}
	n, err := input.Write(data)
	if errors.Is(err, os.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, windows.ERROR_BROKEN_PIPE) ||
		errors.Is(err, windows.ERROR_INVALID_HANDLE) ||
		errors.Is(err, windows.ERROR_NO_DATA) ||
		errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		return n, io.ErrClosedPipe
	}
	return n, err
}

func (t *transport) Resize(ctx context.Context, columns, rows uint16) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	size, err := conPTYSize(columns, rows)
	if err != nil {
		return err
	}

	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.console == 0 {
		return io.ErrClosedPipe
	}
	if err := windows.ResizePseudoConsole(t.console, size); err != nil {
		return fmt.Errorf("resize ConPTY: %w", err)
	}
	return nil
}

func (t *transport) Signal(ctx context.Context, signal port.TerminalSignal) error {
	if err := ctx.Err(); err != nil && signal != port.SignalKill {
		return err
	}
	switch signal {
	case port.SignalHangup, port.SignalInterrupt:
		_, err := t.Write([]byte{0x03})
		if signal == port.SignalHangup && (errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrClosed)) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("send %s to ConPTY: %w", signal, err)
		}
		return nil
	case port.SignalTerminate:
		return t.terminateJob(signal, terminateExitCode)
	case port.SignalKill:
		return t.terminateJob(signal, killExitCode)
	default:
		return fmt.Errorf("unsupported terminal signal %q", signal)
	}
}

func (t *transport) Wait() (port.ExitStatus, error) {
	t.waitOnce.Do(func() {
		t.waitState, t.waitErr = t.wait()
	})
	return t.waitState, t.waitErr
}

func (t *transport) wait() (port.ExitStatus, error) {
	t.stateMu.Lock()
	process := t.process
	t.stateMu.Unlock()
	if process == 0 {
		return port.ExitStatus{}, errors.New("local shell process handle is closed")
	}

	event, err := windows.WaitForSingleObject(process, windows.INFINITE)
	if err != nil {
		t.releaseAfterWait()
		return port.ExitStatus{}, fmt.Errorf("wait for local shell: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		t.releaseAfterWait()
		return port.ExitStatus{}, fmt.Errorf("wait for local shell returned event %#x", event)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(process, &exitCode); err != nil {
		t.releaseAfterWait()
		return port.ExitStatus{}, fmt.Errorf("read local shell exit code: %w", err)
	}

	t.stateMu.Lock()
	signal := t.terminationSignal
	t.stateMu.Unlock()
	t.releaseAfterWait()
	return port.ExitStatus{Code: int(exitCode), Signal: string(signal)}, nil
}

func (t *transport) Close() error {
	t.closeOnce.Do(func() {
		t.closeErr = errors.Join(t.closeInput(), t.closeOutput())
	})
	return t.closeErr
}

func (t *transport) terminateJob(signal port.TerminalSignal, exitCode uint32) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.job == 0 {
		return nil
	}
	if t.process != 0 {
		event, err := windows.WaitForSingleObject(t.process, 0)
		if err == nil && event == windows.WAIT_OBJECT_0 {
			return nil
		}
	}
	if err := windows.TerminateJobObject(t.job, exitCode); err != nil {
		return fmt.Errorf("%s local shell process tree: %w", signal, err)
	}
	t.terminationSignal = signal
	return nil
}

func (t *transport) releaseAfterWait() {
	t.stateMu.Lock()
	job := t.job
	console := t.console
	process := t.process
	t.job = 0
	t.console = 0
	t.process = 0
	t.stateMu.Unlock()

	_ = t.closeInput()
	if job != 0 {
		_ = windows.TerminateJobObject(job, killExitCode)
	}
	if console != 0 {
		closed := make(chan struct{})
		go func() {
			windows.ClosePseudoConsole(console)
			close(closed)
		}()
		timer := time.NewTimer(conPTYCloseDrainTimeout)
		select {
		case <-closed:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			_ = t.closeOutput()
			<-closed
		}
	}
	if process != 0 {
		_ = windows.CloseHandle(process)
	}
	if job != 0 {
		_ = windows.CloseHandle(job)
	}
}

func (t *transport) closeInput() error {
	t.stateMu.Lock()
	input := t.input
	t.input = nil
	t.stateMu.Unlock()
	if input == nil {
		return nil
	}
	return input.Close()
}

func (t *transport) closeOutput() error {
	t.stateMu.Lock()
	output := t.output
	t.output = nil
	t.stateMu.Unlock()
	if output == nil {
		return nil
	}
	return output.Close()
}

func ensureConPTYAvailable() error {
	for _, required := range requiredConPTYProcedures {
		if err := required.procedure.Find(); err != nil {
			return fmt.Errorf("%s is unavailable; ConPTY requires Windows 10 version 1809 or newer: %w", required.name, err)
		}
	}
	return nil
}

func attachPseudoConsole(attributes *windows.ProcThreadAttributeListContainer, console windows.Handle) error {
	succeeded, _, callErr := updateProcThreadAttribute.Call(
		uintptr(unsafe.Pointer(attributes.List())),
		0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		uintptr(console),
		unsafe.Sizeof(console),
		0,
		0,
	)
	if succeeded != 0 {
		return nil
	}
	if callErr == windows.ERROR_SUCCESS {
		return errors.New("UpdateProcThreadAttribute failed without a Windows error")
	}
	return callErr
}

func conPTYSize(columns, rows uint16) (windows.Coord, error) {
	if columns == 0 || columns > 32767 || rows == 0 || rows > 32767 {
		return windows.Coord{}, fmt.Errorf("invalid ConPTY size %dx%d", columns, rows)
	}
	return windows.Coord{X: int16(columns), Y: int16(rows)}, nil
}

func newTerminalJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create local shell cleanup job: %w", err)
	}
	information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("configure local shell cleanup job: %w", err)
	}
	return job, nil
}

func closeHandle(handle *windows.Handle) {
	if *handle == 0 {
		return
	}
	_ = windows.CloseHandle(*handle)
	*handle = 0
}

var _ port.TerminalTransport = (*transport)(nil)
