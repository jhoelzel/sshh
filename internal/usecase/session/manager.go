package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"shh-h/internal/apperror"
	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

const (
	// MaximumOpenSessions bounds terminal transports, process handles, and the
	// goroutines attached to sessions that have not yet been closed by the user.
	MaximumOpenSessions = 64

	maxOutputChunk      = 64 * 1024
	maxUnackedBytes     = 1024 * 1024
	activationTimeout   = 10 * time.Second
	gracefulCloseWindow = 700 * time.Millisecond
	defaultLogMaxBytes  = 10 * 1024 * 1024
	defaultLogRotations = 5
)

type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateClosing  State = "closing"
	StateExited   State = "exited"
	StateFailed   State = "failed"
	StateClosed   State = "closed"
)

type Session struct {
	ID         string `json:"id"`
	Generation uint64 `json:"generation"`
	LeaseID    string `json:"leaseId"`
	ProfileID  string `json:"profileId"`
	Title      string `json:"title"`
	State      State  `json:"state"`
	Columns    uint16 `json:"columns"`
	Rows       uint16 `json:"rows"`
	StartedAt  string `json:"startedAt"`
}

type OutputChunk struct {
	LeaseID    string
	SessionID  string
	Generation uint64
	Sequence   uint64
	EndOffset  uint64
	Data       []byte
	Final      bool
}

type StateEvent struct {
	LeaseID    string `json:"leaseId"`
	SessionID  string `json:"sessionId"`
	Generation uint64 `json:"generation"`
	Title      string `json:"title"`
	State      State  `json:"state"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Signal     string `json:"signal,omitempty"`
	Message    string `json:"message,omitempty"`
}

type Sink interface {
	PublishOutput(OutputChunk)
	PublishState(StateEvent)
}

type LogSink interface {
	PublishSessionLog(SessionLogStatus)
}

type SessionLogStatus struct {
	LeaseID        string `json:"leaseId"`
	SessionID      string `json:"sessionId"`
	Generation     uint64 `json:"generation"`
	Active         bool   `json:"active"`
	Path           string `json:"path"`
	BytesWritten   int64  `json:"bytesWritten"`
	TimestampLines bool   `json:"timestampLines"`
	StartedAt      string `json:"startedAt"`
	StoppedAt      string `json:"stoppedAt"`
	Message        string `json:"message"`
}

// Diagnostics is a content-free flow-control snapshot for native performance
// verification. It deliberately exposes counters only, never terminal bytes.
type Diagnostics struct {
	SessionID               string `json:"sessionId"`
	Generation              uint64 `json:"generation"`
	NextSequence            uint64 `json:"nextSequence"`
	EmittedBytes            uint64 `json:"emittedBytes"`
	AcknowledgedSequence    uint64 `json:"acknowledgedSequence"`
	AcknowledgedBytes       uint64 `json:"acknowledgedBytes"`
	UnacknowledgedBytes     uint64 `json:"unacknowledgedBytes"`
	PendingChunks           int    `json:"pendingChunks"`
	PeakUnacknowledgedBytes uint64 `json:"peakUnacknowledgedBytes"`
	PeakPendingChunks       int    `json:"peakPendingChunks"`
	MaximumUnacknowledged   uint64 `json:"maximumUnacknowledged"`
}

type Manager struct {
	mu                  sync.RWMutex
	factory             port.TerminalFactory
	sshFactory          port.SSHTerminalFactory
	logFactory          port.SessionLogFactory
	sink                Sink
	runtimes            map[string]*runtimeSession
	openingSessions     int
	maximumOpenSessions int
	output              *outputDispatcher
}

type outputMeta struct {
	sequence  uint64
	endOffset uint64
}

type runtimeSession struct {
	mu            sync.RWMutex
	session       Session
	transport     port.TerminalTransport
	ctx           context.Context
	cancel        context.CancelFunc
	activate      chan struct{}
	activateOnce  sync.Once
	waitDone      chan struct{}
	outputDone    chan struct{}
	closeComplete chan struct{}
	closeOnce     sync.Once
	lastInput     uint64
	flowMu        sync.Mutex
	nextSequence  uint64
	emittedBytes  uint64
	ackedBytes    uint64
	ackedSequence uint64
	pending       []outputMeta
	peakUnacked   uint64
	peakPending   int
	flowClosed    bool
	flowWake      chan struct{}
	logMu         sync.Mutex
	logger        port.SessionLog
	logStatus     SessionLogStatus
}

func NewManager(factory port.TerminalFactory) *Manager {
	return newManager(factory, MaximumOpenSessions)
}

func newManager(factory port.TerminalFactory, maximumOpenSessions int) *Manager {
	return &Manager{
		factory:             factory,
		runtimes:            make(map[string]*runtimeSession),
		maximumOpenSessions: maximumOpenSessions,
	}
}

func (m *Manager) SetSink(sink Sink) {
	m.mu.Lock()
	m.sink = sink
	m.mu.Unlock()
}

func (m *Manager) SetSSHFactory(factory port.SSHTerminalFactory) {
	m.mu.Lock()
	m.sshFactory = factory
	m.mu.Unlock()
}

func (m *Manager) SetLogFactory(factory port.SessionLogFactory) {
	m.mu.Lock()
	m.logFactory = factory
	m.mu.Unlock()
}

func (m *Manager) OpenLocal(ctx context.Context, leaseID string, selected profile.Profile, columns, rows uint16) (Session, error) {
	if leaseID == "" {
		return Session{}, apperror.New(apperror.CodeStale, "A current frontend lease is required.")
	}
	if selected.Protocol != profile.ProtocolLocal {
		return Session{}, apperror.New(
			apperror.CodeInvalidArgument, fmt.Sprintf("Profile %q is not a local terminal.", selected.ID),
		)
	}
	if err := validateSize(columns, rows); err != nil {
		return Session{}, err
	}
	if err := m.reserveSessionSlot(); err != nil {
		return Session{}, err
	}
	registered := false
	defer func() {
		if !registered {
			m.releaseSessionSlotReservation()
		}
	}()

	id, err := newID()
	if err != nil {
		return Session{}, err
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	environment := make([]string, 0, len(selected.Environment)+1)
	for key, value := range selected.Environment {
		environment = append(environment, key+"="+value)
	}
	sort.Strings(environment)
	environment = append(environment, "SHHH_SESSION_ID="+id)

	transport, err := m.factory.Open(runtimeCtx, port.TerminalSpec{
		Command:          selected.Shell,
		Arguments:        selected.Arguments,
		WorkingDirectory: selected.WorkingDirectory,
		Environment:      environment,
		Columns:          columns,
		Rows:             rows,
	})
	if err != nil {
		cancel()
		return Session{}, err
	}
	opened := m.registerRuntime(runtimeCtx, cancel, id, leaseID, selected, columns, rows, transport)
	registered = true
	return opened, nil
}

func (m *Manager) OpenSSH(ctx context.Context, leaseID string, selected profile.Profile, columns, rows uint16, credentials port.SSHCredentials) (Session, error) {
	defer clear(credentials.Password)
	defer clear(credentials.Passphrase)

	if leaseID == "" {
		return Session{}, apperror.New(apperror.CodeStale, "A current frontend lease is required.")
	}
	if selected.Protocol != profile.ProtocolSSH {
		return Session{}, apperror.New(
			apperror.CodeInvalidArgument, fmt.Sprintf("Profile %q is not an SSH terminal.", selected.ID),
		)
	}
	if err := selected.Validate(); err != nil {
		return Session{}, err
	}
	if err := validateSize(columns, rows); err != nil {
		return Session{}, err
	}

	m.mu.RLock()
	factory := m.sshFactory
	m.mu.RUnlock()
	if factory == nil {
		return Session{}, apperror.New(apperror.CodeUnavailable, "SSH terminal support is unavailable.")
	}
	if err := m.reserveSessionSlot(); err != nil {
		return Session{}, err
	}
	registered := false
	defer func() {
		if !registered {
			m.releaseSessionSlotReservation()
		}
	}()

	id, err := newID()
	if err != nil {
		return Session{}, err
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	transport, err := factory.OpenSSH(runtimeCtx, port.SSHTerminalSpec{
		ProfileID: selected.ID, Host: selected.Host, Port: selected.Port, Username: selected.Username,
		Authentication: selected.Authentication, IdentityFile: selected.IdentityFile,
		Credentials: credentials, Columns: columns, Rows: rows,
	})
	if err != nil {
		cancel()
		return Session{}, err
	}
	opened := m.registerRuntime(runtimeCtx, cancel, id, leaseID, selected, columns, rows, transport)
	registered = true
	return opened, nil
}

func (m *Manager) registerRuntime(runtimeCtx context.Context, cancel context.CancelFunc, id, leaseID string, selected profile.Profile, columns, rows uint16, transport port.TerminalTransport) Session {
	snapshot := Session{
		ID: id, Generation: 1, LeaseID: leaseID, ProfileID: selected.ID,
		Title: selected.Name, State: StateStarting, Columns: columns, Rows: rows,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	runtime := &runtimeSession{
		session: snapshot, transport: transport, ctx: runtimeCtx, cancel: cancel,
		activate: make(chan struct{}), waitDone: make(chan struct{}),
		outputDone: make(chan struct{}), closeComplete: make(chan struct{}),
		flowWake: make(chan struct{}, 1),
	}

	m.mu.Lock()
	m.openingSessions--
	if m.output == nil {
		m.output = newOutputDispatcher(m.emitOutput)
	}
	m.runtimes[id] = runtime
	m.mu.Unlock()
	m.publishState(runtime, nil, "")

	go m.outputPump(runtime)
	go m.waitForProcess(runtime)
	go m.enforceActivation(runtime)
	return snapshot
}

func (m *Manager) reserveSessionSlot() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.runtimes)+m.openingSessions >= m.maximumOpenSessions {
		return apperror.New(
			apperror.CodeResourceExhausted,
			fmt.Sprintf("Open terminal limit of %d reached. Close a terminal and try again.", m.maximumOpenSessions),
		)
	}
	m.openingSessions++
	return nil
}

func (m *Manager) releaseSessionSlotReservation() {
	m.mu.Lock()
	m.openingSessions--
	m.mu.Unlock()
}

func (m *Manager) Activate(leaseID, sessionID string, generation uint64) error {
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return err
	}

	runtime.activateOnce.Do(func() { close(runtime.activate) })
	runtime.mu.Lock()
	if runtime.session.State == StateStarting {
		runtime.session.State = StateRunning
	}
	runtime.mu.Unlock()
	m.publishState(runtime, nil, "")
	return nil
}

func (m *Manager) Write(leaseID, sessionID string, generation, inputSequence uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if len(data) > maxOutputChunk {
		return apperror.New(
			apperror.CodeInvalidArgument, fmt.Sprintf("Terminal input exceeds %d bytes.", maxOutputChunk),
		)
	}
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return err
	}

	runtime.mu.Lock()
	if inputSequence <= runtime.lastInput {
		runtime.mu.Unlock()
		return nil
	}
	if inputSequence != runtime.lastInput+1 {
		expected := runtime.lastInput + 1
		runtime.mu.Unlock()
		return apperror.New(
			apperror.CodeStale, fmt.Sprintf("Terminal input sequence gap: expected %d, got %d.", expected, inputSequence),
		)
	}
	if runtime.session.State != StateRunning {
		state := runtime.session.State
		runtime.mu.Unlock()
		return apperror.New(apperror.CodeConflict, fmt.Sprintf("Terminal session is %s.", state))
	}
	runtime.lastInput = inputSequence
	runtime.mu.Unlock()

	if _, err := runtime.transport.Write(data); err != nil {
		return apperror.Wrap(
			apperror.CodeUnavailable, "write terminal input", "Terminal input could not be written.", err,
		)
	}
	return nil
}

func (m *Manager) Resize(leaseID, sessionID string, generation uint64, columns, rows uint16) error {
	if err := validateSize(columns, rows); err != nil {
		return err
	}
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return err
	}
	if err := runtime.transport.Resize(runtime.ctx, columns, rows); err != nil {
		return err
	}

	runtime.mu.Lock()
	runtime.session.Columns = columns
	runtime.session.Rows = rows
	runtime.mu.Unlock()
	return nil
}

func (m *Manager) Acknowledge(leaseID, sessionID string, generation, throughSequence, bytesConsumed uint64) error {
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return err
	}
	return runtime.acknowledge(throughSequence, bytesConsumed)
}

func (m *Manager) Diagnostics(leaseID, sessionID string, generation uint64) (Diagnostics, error) {
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return Diagnostics{}, err
	}
	return runtime.diagnostics(), nil
}

func (m *Manager) StartLogging(leaseID, sessionID string, generation uint64, timestampLines bool) (SessionLogStatus, error) {
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return SessionLogStatus{}, err
	}
	m.mu.RLock()
	factory := m.logFactory
	m.mu.RUnlock()
	if factory == nil {
		return SessionLogStatus{}, apperror.New(apperror.CodeUnavailable, "Session logging is unavailable.")
	}

	runtime.logMu.Lock()
	if runtime.logger != nil {
		status := runtime.logStatus
		runtime.logMu.Unlock()
		return status, apperror.New(apperror.CodeConflict, "Session logging is already active.")
	}
	runtime.mu.RLock()
	state := runtime.session.State
	title := runtime.session.Title
	if state != StateRunning {
		runtime.mu.RUnlock()
		runtime.logMu.Unlock()
		return SessionLogStatus{}, fmt.Errorf("session is %s", state)
	}
	logger, err := factory.Open(port.SessionLogSpec{
		SessionID: sessionID, Title: title, TimestampLines: timestampLines,
		MaxBytes: defaultLogMaxBytes, RotationFiles: defaultLogRotations,
	})
	runtime.mu.RUnlock()
	if err != nil {
		runtime.logMu.Unlock()
		return SessionLogStatus{}, err
	}
	runtime.logger = logger
	runtime.logStatus = SessionLogStatus{
		LeaseID: leaseID, SessionID: sessionID, Generation: generation, Active: true,
		Path: logger.Path(), TimestampLines: timestampLines,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	status := runtime.logStatus
	runtime.logMu.Unlock()
	m.publishLog(status)
	return status, nil
}

func (m *Manager) StopLogging(leaseID, sessionID string, generation uint64) (SessionLogStatus, error) {
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return SessionLogStatus{}, err
	}
	status, active, closeErr := m.stopLogging(runtime, "")
	if !active {
		return status, apperror.New(apperror.CodeConflict, "Session logging is not active.")
	}
	if closeErr != nil {
		return status, closeErr
	}
	return status, nil
}

func (m *Manager) LoggingStatus(leaseID, sessionID string, generation uint64) (SessionLogStatus, error) {
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return SessionLogStatus{}, err
	}
	runtime.logMu.Lock()
	defer runtime.logMu.Unlock()
	status := runtime.logStatus
	if runtime.logger != nil {
		status.BytesWritten = runtime.logger.BytesWritten()
	}
	if status.SessionID == "" {
		status = SessionLogStatus{LeaseID: leaseID, SessionID: sessionID, Generation: generation}
	}
	return status, nil
}

func (m *Manager) Close(leaseID, sessionID string, generation uint64) error {
	runtime, err := m.runtime(leaseID, sessionID, generation)
	if err != nil {
		return err
	}
	m.closeRuntime(runtime)

	var dispatcher *outputDispatcher
	m.mu.Lock()
	if current := m.runtimes[sessionID]; current == runtime {
		delete(m.runtimes, sessionID)
	}
	if len(m.runtimes) == 0 {
		dispatcher = m.output
		m.output = nil
	}
	m.mu.Unlock()
	if dispatcher != nil {
		dispatcher.Close()
	}
	return nil
}

func (m *Manager) CloseLease(leaseID string) {
	runtimes := m.runtimesForLease(leaseID)
	var wait sync.WaitGroup
	for _, runtime := range runtimes {
		wait.Add(1)
		go func(runtime *runtimeSession) {
			defer wait.Done()
			m.closeRuntime(runtime)
		}(runtime)
	}
	wait.Wait()
	var dispatcher *outputDispatcher
	m.mu.Lock()
	for _, runtime := range runtimes {
		if m.runtimes[runtime.session.ID] == runtime {
			delete(m.runtimes, runtime.session.ID)
		}
	}
	if len(m.runtimes) == 0 {
		dispatcher = m.output
		m.output = nil
	}
	m.mu.Unlock()
	if dispatcher != nil {
		dispatcher.Close()
	}
}

func (m *Manager) Shutdown() {
	m.mu.RLock()
	runtimes := make([]*runtimeSession, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runtimes = append(runtimes, runtime)
	}
	m.mu.RUnlock()

	var wait sync.WaitGroup
	for _, runtime := range runtimes {
		wait.Add(1)
		go func(runtime *runtimeSession) {
			defer wait.Done()
			m.closeRuntime(runtime)
		}(runtime)
	}
	wait.Wait()
	var dispatcher *outputDispatcher
	m.mu.Lock()
	clear(m.runtimes)
	dispatcher = m.output
	m.output = nil
	m.mu.Unlock()
	if dispatcher != nil {
		dispatcher.Close()
	}
}

func (m *Manager) LiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, runtime := range m.runtimes {
		runtime.mu.RLock()
		state := runtime.session.State
		runtime.mu.RUnlock()
		if state == StateStarting || state == StateRunning || state == StateClosing {
			count++
		}
	}
	return count
}

func (m *Manager) outputPump(runtime *runtimeSession) {
	defer close(runtime.outputDone)
	select {
	case <-runtime.activate:
	case <-runtime.ctx.Done():
		return
	}

	buffer := make([]byte, maxOutputChunk)
	for {
		n, err := runtime.transport.Read(buffer)
		if n > 0 {
			data := append([]byte(nil), buffer[:n]...)
			m.writeLog(runtime, data)
			if publishErr := m.publishOutput(runtime, data, false); publishErr != nil {
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && runtime.ctx.Err() == nil {
				_ = m.publishOutput(runtime, nil, true)
			}
			return
		}
	}
}

func (m *Manager) waitForProcess(runtime *runtimeSession) {
	status, waitErr := runtime.transport.Wait()
	select {
	case <-runtime.outputDone:
	case <-time.After(2 * time.Second):
		_ = runtime.transport.Close()
	}
	runtime.cancel()
	_ = runtime.transport.Close()
	runtime.closeFlow()

	runtime.mu.Lock()
	state := runtime.session.State
	if state == StateClosing {
		runtime.session.State = StateClosed
	} else if waitErr != nil {
		runtime.session.State = StateFailed
	} else {
		runtime.session.State = StateExited
	}
	runtime.mu.Unlock()
	m.stopLogging(runtime, "")

	if waitErr != nil {
		m.publishState(runtime, nil, waitErr.Error())
	} else {
		m.publishState(runtime, &status, "")
	}
	close(runtime.waitDone)
}

func (m *Manager) enforceActivation(runtime *runtimeSession) {
	timer := time.NewTimer(activationTimeout)
	defer timer.Stop()
	select {
	case <-runtime.activate:
		return
	case <-runtime.waitDone:
		return
	case <-timer.C:
		m.closeRuntime(runtime)
	}
}

func (m *Manager) publishOutput(runtime *runtimeSession, data []byte, final bool) error {
	sequence, endOffset, err := runtime.reserveOutput(len(data))
	if err != nil {
		return err
	}
	snapshot := runtime.snapshot()
	chunk := OutputChunk{
		LeaseID: snapshot.LeaseID, SessionID: snapshot.ID, Generation: snapshot.Generation,
		Sequence: sequence, EndOffset: endOffset, Data: data, Final: final,
	}
	m.mu.RLock()
	dispatcher := m.output
	m.mu.RUnlock()
	if dispatcher == nil {
		return apperror.New(apperror.CodeUnavailable, "Terminal event delivery is unavailable.")
	}
	return dispatcher.Dispatch(runtime.ctx, chunk)
}

func (m *Manager) emitOutput(chunk OutputChunk) error {
	m.mu.RLock()
	sink := m.sink
	m.mu.RUnlock()
	if sink == nil {
		return apperror.New(apperror.CodeUnavailable, "Terminal event delivery is unavailable.")
	}
	sink.PublishOutput(chunk)
	return nil
}

func (m *Manager) publishState(runtime *runtimeSession, status *port.ExitStatus, message string) {
	snapshot := runtime.snapshot()
	event := StateEvent{
		LeaseID: snapshot.LeaseID, SessionID: snapshot.ID, Generation: snapshot.Generation,
		Title: snapshot.Title, State: snapshot.State, Message: message,
	}
	if status != nil {
		exitCode := status.Code
		event.ExitCode = &exitCode
		event.Signal = status.Signal
	}
	m.mu.RLock()
	sink := m.sink
	m.mu.RUnlock()
	if sink != nil {
		sink.PublishState(event)
	}
}

func (m *Manager) writeLog(runtime *runtimeSession, data []byte) {
	runtime.logMu.Lock()
	if runtime.logger == nil {
		runtime.logMu.Unlock()
		return
	}
	_, err := runtime.logger.Write(data)
	if err == nil {
		runtime.logStatus.BytesWritten = runtime.logger.BytesWritten()
		runtime.logMu.Unlock()
		return
	}
	_ = runtime.logger.Close()
	runtime.logStatus.BytesWritten = runtime.logger.BytesWritten()
	runtime.logStatus.Active = false
	runtime.logStatus.StoppedAt = time.Now().UTC().Format(time.RFC3339Nano)
	runtime.logStatus.Message = err.Error()
	runtime.logger = nil
	status := runtime.logStatus
	runtime.logMu.Unlock()
	m.publishLog(status)
}

func (m *Manager) stopLogging(runtime *runtimeSession, message string) (SessionLogStatus, bool, error) {
	runtime.logMu.Lock()
	if runtime.logger == nil {
		status := runtime.logStatus
		runtime.logMu.Unlock()
		return status, false, nil
	}
	logger := runtime.logger
	runtime.logStatus.BytesWritten = logger.BytesWritten()
	runtime.logStatus.Active = false
	runtime.logStatus.StoppedAt = time.Now().UTC().Format(time.RFC3339Nano)
	runtime.logStatus.Message = message
	runtime.logger = nil
	closeErr := logger.Close()
	if closeErr != nil && runtime.logStatus.Message == "" {
		runtime.logStatus.Message = closeErr.Error()
	}
	status := runtime.logStatus
	runtime.logMu.Unlock()
	m.publishLog(status)
	return status, true, closeErr
}

func (m *Manager) publishLog(status SessionLogStatus) {
	m.mu.RLock()
	sink, ok := m.sink.(LogSink)
	m.mu.RUnlock()
	if ok {
		sink.PublishSessionLog(status)
	}
}

func (m *Manager) closeRuntime(runtime *runtimeSession) {
	runtime.closeOnce.Do(func() {
		m.closeRuntimeOnce(runtime)
		close(runtime.closeComplete)
	})
	<-runtime.closeComplete
}

func (m *Manager) closeRuntimeOnce(runtime *runtimeSession) {
	defer m.stopLogging(runtime, "")
	runtime.mu.Lock()
	state := runtime.session.State
	if state == StateClosed {
		runtime.mu.Unlock()
		return
	}
	if state == StateExited || state == StateFailed {
		runtime.session.State = StateClosed
		runtime.mu.Unlock()
		runtime.cancel()
		runtime.closeFlow()
		_ = runtime.transport.Close()
		m.publishState(runtime, nil, "")
		return
	}
	runtime.session.State = StateClosing
	runtime.mu.Unlock()
	m.publishState(runtime, nil, "")

	runtime.activateOnce.Do(func() { close(runtime.activate) })
	runtime.cancel()
	runtime.closeFlow()
	_ = runtime.transport.Signal(context.Background(), port.SignalHangup)
	_ = runtime.transport.Close()

	if waitFor(runtime.waitDone, gracefulCloseWindow) {
		return
	}
	_ = runtime.transport.Signal(context.Background(), port.SignalTerminate)
	if waitFor(runtime.waitDone, gracefulCloseWindow) {
		return
	}
	_ = runtime.transport.Signal(context.Background(), port.SignalKill)
	_ = waitFor(runtime.waitDone, gracefulCloseWindow)
}

func (m *Manager) runtime(leaseID, sessionID string, generation uint64) (*runtimeSession, error) {
	m.mu.RLock()
	runtime := m.runtimes[sessionID]
	m.mu.RUnlock()
	if runtime == nil {
		return nil, apperror.New(apperror.CodeNotFound, "Terminal session was not found.")
	}

	snapshot := runtime.snapshot()
	if snapshot.LeaseID != leaseID {
		return nil, apperror.New(apperror.CodeStale, "Terminal session belongs to another frontend lease.")
	}
	if snapshot.Generation != generation {
		return nil, apperror.New(apperror.CodeStale, "Terminal session generation is stale.")
	}
	return runtime, nil
}

func (m *Manager) runtimesForLease(leaseID string) []*runtimeSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*runtimeSession, 0)
	for _, runtime := range m.runtimes {
		if runtime.snapshot().LeaseID == leaseID {
			result = append(result, runtime)
		}
	}
	return result
}

func (r *runtimeSession) snapshot() Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.session
}

func (r *runtimeSession) reserveOutput(byteCount int) (uint64, uint64, error) {
	for {
		r.flowMu.Lock()
		if r.flowClosed {
			r.flowMu.Unlock()
			return 0, 0, context.Canceled
		}
		if r.emittedBytes-r.ackedBytes+uint64(byteCount) <= maxUnackedBytes {
			r.nextSequence++
			r.emittedBytes += uint64(byteCount)
			meta := outputMeta{sequence: r.nextSequence, endOffset: r.emittedBytes}
			r.pending = append(r.pending, meta)
			unacked := r.emittedBytes - r.ackedBytes
			if unacked > r.peakUnacked {
				r.peakUnacked = unacked
			}
			if len(r.pending) > r.peakPending {
				r.peakPending = len(r.pending)
			}
			r.flowMu.Unlock()
			return meta.sequence, meta.endOffset, nil
		}
		r.flowMu.Unlock()

		select {
		case <-r.ctx.Done():
			return 0, 0, r.ctx.Err()
		case <-r.flowWake:
		}
	}
}

func (r *runtimeSession) diagnostics() Diagnostics {
	snapshot := r.snapshot()
	r.flowMu.Lock()
	defer r.flowMu.Unlock()
	return Diagnostics{
		SessionID:               snapshot.ID,
		Generation:              snapshot.Generation,
		NextSequence:            r.nextSequence,
		EmittedBytes:            r.emittedBytes,
		AcknowledgedSequence:    r.ackedSequence,
		AcknowledgedBytes:       r.ackedBytes,
		UnacknowledgedBytes:     r.emittedBytes - r.ackedBytes,
		PendingChunks:           len(r.pending),
		PeakUnacknowledgedBytes: r.peakUnacked,
		PeakPendingChunks:       r.peakPending,
		MaximumUnacknowledged:   maxUnackedBytes,
	}
}

func (r *runtimeSession) acknowledge(sequence, bytesConsumed uint64) error {
	r.flowMu.Lock()
	defer r.flowMu.Unlock()

	if sequence <= r.ackedSequence {
		return nil
	}
	index := -1
	var expected uint64
	for i, meta := range r.pending {
		if meta.sequence == sequence {
			index = i
			expected = meta.endOffset
			break
		}
	}
	if index < 0 {
		return apperror.New(apperror.CodeStale, fmt.Sprintf("Unknown terminal output sequence %d.", sequence))
	}
	if bytesConsumed != expected {
		return apperror.New(
			apperror.CodeStale,
			fmt.Sprintf("Terminal output acknowledgement mismatch: expected %d, got %d.", expected, bytesConsumed),
		)
	}

	r.ackedSequence = sequence
	r.ackedBytes = expected
	r.pending = append([]outputMeta(nil), r.pending[index+1:]...)
	notify(r.flowWake)
	return nil
}

func (r *runtimeSession) closeFlow() {
	r.flowMu.Lock()
	r.flowClosed = true
	r.flowMu.Unlock()
	notify(r.flowWake)
}

func validateSize(columns, rows uint16) error {
	if columns < 2 || columns > 500 || rows < 1 || rows > 300 {
		return apperror.New(
			apperror.CodeInvalidArgument, fmt.Sprintf("Invalid terminal size %dx%d.", columns, rows),
		)
	}
	return nil
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func notify(channel chan struct{}) {
	select {
	case channel <- struct{}{}:
	default:
	}
}

func waitFor(channel <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-channel:
		return true
	case <-timer.C:
		return false
	}
}
