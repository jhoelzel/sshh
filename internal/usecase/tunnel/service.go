package tunnel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"shh-h/internal/domain/profile"
	tunneldomain "shh-h/internal/domain/tunnel"
	"shh-h/internal/port"
)

const (
	initialRetryDelay = time.Second
	maximumRetryDelay = 30 * time.Second
	maximumRelays     = 256
)

type Repository interface {
	LoadTunnels() ([]tunneldomain.Config, error)
	SaveTunnels([]tunneldomain.Config) error
}

type ProfileFinder interface {
	Find(string) (profile.Profile, bool)
}

type Sink interface {
	PublishTunnel(tunneldomain.Snapshot)
}

type Service struct {
	mu       sync.RWMutex
	repo     Repository
	profiles ProfileFinder
	factory  port.SSHConnectionFactory
	configs  []tunneldomain.Config
	runtimes map[string]*runtimeTunnel
	sink     Sink
}

type runtimeTunnel struct {
	mu          sync.RWMutex
	attemptMu   sync.Mutex
	snapshot    tunneldomain.Snapshot
	config      tunneldomain.Config
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	listener    net.Listener
	connection  port.SSHConnection
	connections map[net.Conn]struct{}
	relays      chan struct{}
	accepting   sync.WaitGroup
	wait        sync.WaitGroup
}

func NewService(repo Repository, profiles ProfileFinder, factory port.SSHConnectionFactory) (*Service, error) {
	configs, err := repo.LoadTunnels()
	if err != nil {
		return nil, err
	}
	return &Service{
		repo: repo, profiles: profiles, factory: factory,
		configs: append([]tunneldomain.Config{}, configs...), runtimes: make(map[string]*runtimeTunnel),
	}, nil
}

func (s *Service) SetSink(sink Sink) {
	s.mu.Lock()
	s.sink = sink
	s.mu.Unlock()
}

func (s *Service) List() []tunneldomain.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]tunneldomain.Config{}, s.configs...)
}

func (s *Service) Find(id string) (tunneldomain.Config, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	index := configIndex(s.configs, id)
	if index < 0 {
		return tunneldomain.Config{}, false
	}
	return s.configs[index], true
}

func (s *Service) Create(candidate tunneldomain.Config) (tunneldomain.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := newID()
	if err != nil {
		return tunneldomain.Config{}, err
	}
	now := time.Now().UTC()
	candidate.ID = id
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := s.validateConfig(candidate, ""); err != nil {
		return tunneldomain.Config{}, err
	}
	next := append(append([]tunneldomain.Config{}, s.configs...), candidate)
	if err := s.repo.SaveTunnels(next); err != nil {
		return tunneldomain.Config{}, err
	}
	s.configs = next
	return candidate, nil
}

func (s *Service) Update(candidate tunneldomain.Config) (tunneldomain.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := configIndex(s.configs, strings.TrimSpace(candidate.ID))
	if index < 0 {
		return tunneldomain.Config{}, errors.New("tunnel not found")
	}
	if current := s.runtimes[candidate.ID]; current != nil && current.snapshotValue().Live() {
		return tunneldomain.Config{}, errors.New("stop the tunnel before editing it")
	}
	now := time.Now().UTC()
	candidate.ID = s.configs[index].ID
	candidate.CreatedAt = s.configs[index].CreatedAt
	candidate.UpdatedAt = now
	candidate = candidate.WithDefaults(now)
	if err := s.validateConfig(candidate, candidate.ID); err != nil {
		return tunneldomain.Config{}, err
	}
	next := append([]tunneldomain.Config{}, s.configs...)
	next[index] = candidate
	if err := s.repo.SaveTunnels(next); err != nil {
		return tunneldomain.Config{}, err
	}
	s.configs = next
	return candidate, nil
}

func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := configIndex(s.configs, id)
	if index < 0 {
		return errors.New("tunnel not found")
	}
	if current := s.runtimes[id]; current != nil && current.snapshotValue().Live() {
		return errors.New("stop the tunnel before deleting it")
	}
	next := append([]tunneldomain.Config{}, s.configs[:index]...)
	next = append(next, s.configs[index+1:]...)
	if err := s.repo.SaveTunnels(next); err != nil {
		return err
	}
	s.configs = next
	delete(s.runtimes, id)
	return nil
}

func (s *Service) Start(ctx context.Context, leaseID, configID string, credentials port.SSHCredentials) (tunneldomain.Snapshot, error) {
	if strings.TrimSpace(leaseID) == "" {
		clear(credentials.Password)
		clear(credentials.Passphrase)
		return tunneldomain.Snapshot{}, errors.New("frontend lease is required")
	}
	s.mu.Lock()
	index := configIndex(s.configs, configID)
	if index < 0 {
		s.mu.Unlock()
		clear(credentials.Password)
		clear(credentials.Passphrase)
		return tunneldomain.Snapshot{}, errors.New("tunnel not found")
	}
	config := s.configs[index]
	selected, found := s.profiles.Find(config.ProfileID)
	if !found || selected.Protocol != profile.ProtocolSSH {
		s.mu.Unlock()
		clear(credentials.Password)
		clear(credentials.Passphrase)
		return tunneldomain.Snapshot{}, errors.New("tunnel SSH profile is unavailable")
	}
	if existing := s.runtimes[configID]; existing != nil && existing.snapshotValue().Live() {
		snapshot := existing.snapshotValue()
		s.mu.Unlock()
		clear(credentials.Password)
		clear(credentials.Passphrase)
		if snapshot.LeaseID != leaseID {
			return tunneldomain.Snapshot{}, errors.New("tunnel belongs to another frontend lease")
		}
		return snapshot, nil
	}
	if config.Reconnect && (len(credentials.Password) > 0 || len(credentials.Passphrase) > 0) {
		s.mu.Unlock()
		clear(credentials.Password)
		clear(credentials.Passphrase)
		return tunneldomain.Snapshot{}, errors.New("automatic reconnect requires agent or an unencrypted private key")
	}
	runtimeContext, cancel := context.WithCancel(ctx)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	runtime := &runtimeTunnel{
		config: config, ctx: runtimeContext, cancel: cancel, done: make(chan struct{}),
		connections: make(map[net.Conn]struct{}), relays: make(chan struct{}, maximumRelays),
		snapshot: tunneldomain.Snapshot{ConfigID: configID, LeaseID: leaseID, State: tunneldomain.StateStarting, UpdatedAt: now},
	}
	s.runtimes[configID] = runtime
	s.mu.Unlock()

	secret := port.SSHCredentials{
		Password: append([]byte{}, credentials.Password...), Passphrase: append([]byte{}, credentials.Passphrase...),
	}
	clear(credentials.Password)
	clear(credentials.Passphrase)
	s.publish(runtime.snapshotValue())
	ready := make(chan error, 1)
	go s.run(runtime, selected, secret, ready)
	select {
	case err := <-ready:
		return runtime.snapshotValue(), err
	case <-ctx.Done():
		runtime.cancel()
		runtime.closeAttempt()
		return runtime.snapshotValue(), ctx.Err()
	}
}

func (s *Service) Stop(leaseID, configID string) error {
	s.mu.RLock()
	runtime := s.runtimes[configID]
	s.mu.RUnlock()
	if runtime == nil {
		return nil
	}
	if runtime.snapshotValue().LeaseID != leaseID {
		return errors.New("tunnel belongs to another frontend lease")
	}
	runtime.cancel()
	runtime.closeAttempt()
	<-runtime.done
	return nil
}

func (s *Service) Snapshots(leaseID string) []tunneldomain.Snapshot {
	s.mu.RLock()
	result := make([]tunneldomain.Snapshot, 0, len(s.runtimes))
	for _, runtime := range s.runtimes {
		if snapshot := runtime.snapshotValue(); snapshot.LeaseID == leaseID {
			result = append(result, snapshot)
		}
	}
	s.mu.RUnlock()
	sort.Slice(result, func(left, right int) bool { return result[left].ConfigID < result[right].ConfigID })
	return result
}

func (s *Service) CloseLease(leaseID string) {
	s.mu.RLock()
	runtimes := make([]*runtimeTunnel, 0)
	for _, runtime := range s.runtimes {
		if runtime.snapshotValue().LeaseID == leaseID && runtime.snapshotValue().Live() {
			runtimes = append(runtimes, runtime)
		}
	}
	s.mu.RUnlock()
	for _, runtime := range runtimes {
		runtime.cancel()
		runtime.closeAttempt()
	}
	for _, runtime := range runtimes {
		<-runtime.done
	}
}

func (s *Service) Shutdown() {
	s.mu.RLock()
	runtimes := make([]*runtimeTunnel, 0, len(s.runtimes))
	for _, runtime := range s.runtimes {
		if runtime.snapshotValue().Live() {
			runtimes = append(runtimes, runtime)
		}
	}
	s.mu.RUnlock()
	for _, runtime := range runtimes {
		runtime.cancel()
		runtime.closeAttempt()
	}
	for _, runtime := range runtimes {
		<-runtime.done
	}
}

func (s *Service) LiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, runtime := range s.runtimes {
		if runtime.snapshotValue().Live() {
			count++
		}
	}
	return count
}

func (s *Service) run(runtime *runtimeTunnel, selected profile.Profile, credentials port.SSHCredentials, ready chan<- error) {
	defer close(runtime.done)
	defer clear(credentials.Password)
	defer clear(credentials.Passphrase)
	readySent := false
	retryDelay := initialRetryDelay
	for {
		if err := runtime.ctx.Err(); err != nil {
			runtime.transition(tunneldomain.StateStopped, "", "")
			s.publish(runtime.snapshotValue())
			if !readySent {
				ready <- err
			}
			return
		}
		spec := port.SSHTerminalSpec{
			ProfileID: selected.ID, Host: selected.Host, Port: selected.Port, Username: selected.Username,
			Authentication: selected.Authentication, IdentityFile: selected.IdentityFile,
			Credentials: port.SSHCredentials{
				Password: append([]byte{}, credentials.Password...), Passphrase: append([]byte{}, credentials.Passphrase...),
			},
		}
		connection, listener, err := s.openAttempt(runtime.ctx, runtime.config, spec)
		clear(spec.Credentials.Password)
		clear(spec.Credentials.Passphrase)
		clear(credentials.Password)
		clear(credentials.Passphrase)
		if err == nil {
			runtime.setAttempt(connection, listener)
			runtime.transition(tunneldomain.StateActive, listener.Addr().String(), "")
			s.publish(runtime.snapshotValue())
			if !readySent {
				ready <- nil
				readySent = true
			}
			err = s.serveAttempt(runtime)
		}
		runtime.closeAttempt()
		if errors.Is(err, context.Canceled) || runtime.ctx.Err() != nil {
			runtime.transition(tunneldomain.StateStopped, "", "")
			s.publish(runtime.snapshotValue())
			if !readySent {
				ready <- runtime.ctx.Err()
			}
			return
		}
		if !runtime.config.Reconnect {
			runtime.transition(tunneldomain.StateFailed, "", errorMessage(err))
			s.publish(runtime.snapshotValue())
			if !readySent {
				ready <- err
			}
			return
		}
		runtime.transition(tunneldomain.StateRetrying, "", errorMessage(err))
		s.publish(runtime.snapshotValue())
		if !readySent {
			ready <- nil
			readySent = true
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-runtime.ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		if retryDelay < maximumRetryDelay {
			retryDelay *= 2
			if retryDelay > maximumRetryDelay {
				retryDelay = maximumRetryDelay
			}
		}
	}
}

func (s *Service) openAttempt(ctx context.Context, config tunneldomain.Config, spec port.SSHTerminalSpec) (port.SSHConnection, net.Listener, error) {
	if s.factory == nil {
		return nil, nil, errors.New("SSH tunnel support is unavailable")
	}
	connection, err := s.factory.OpenSSHConnection(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	bindAddress := net.JoinHostPort(strings.Trim(config.BindAddress, "[]"), fmt.Sprintf("%d", config.BindPort))
	var listener net.Listener
	if config.Kind == tunneldomain.KindRemote {
		listener, err = connection.Listen("tcp", bindAddress)
	} else {
		listener, err = (&net.ListenConfig{}).Listen(ctx, "tcp", bindAddress)
	}
	if err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("listen on %s: %w", bindAddress, err)
	}
	return connection, listener, nil
}

func (s *Service) serveAttempt(runtime *runtimeTunnel) error {
	acceptDone := make(chan error, 1)
	waitDone := make(chan error, 1)
	connection := runtime.connectionValue()
	if connection == nil {
		return errors.New("SSH connection is unavailable")
	}
	runtime.accepting.Add(1)
	go func() {
		defer runtime.accepting.Done()
		acceptDone <- s.acceptLoop(runtime)
	}()
	go func() { waitDone <- connection.Wait() }()
	select {
	case <-runtime.ctx.Done():
		return runtime.ctx.Err()
	case err := <-acceptDone:
		return err
	case err := <-waitDone:
		if err == nil {
			return errors.New("SSH connection closed")
		}
		return fmt.Errorf("SSH connection lost: %w", err)
	}
}

func (s *Service) validateConfig(candidate tunneldomain.Config, excludingID string) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	selected, found := s.profiles.Find(candidate.ProfileID)
	if !found || selected.Protocol != profile.ProtocolSSH {
		return errors.New("a valid SSH profile is required")
	}
	name := strings.ToLower(candidate.Name)
	for _, item := range s.configs {
		if item.ID != excludingID && strings.ToLower(item.Name) == name {
			return fmt.Errorf("a tunnel named %q already exists", candidate.Name)
		}
	}
	return nil
}

func (s *Service) publish(snapshot tunneldomain.Snapshot) {
	s.mu.RLock()
	sink := s.sink
	s.mu.RUnlock()
	if sink != nil {
		sink.PublishTunnel(snapshot)
	}
}

func (r *runtimeTunnel) snapshotValue() tunneldomain.Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot
}

func (r *runtimeTunnel) transition(state tunneldomain.State, boundAddress, message string) {
	r.mu.Lock()
	r.snapshot.State = state
	r.snapshot.BoundAddress = boundAddress
	r.snapshot.Message = message
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r.snapshot.UpdatedAt = now
	if state == tunneldomain.StateActive && r.snapshot.StartedAt == "" {
		r.snapshot.StartedAt = now
	}
	r.mu.Unlock()
}

func (r *runtimeTunnel) setAttempt(connection port.SSHConnection, listener net.Listener) {
	r.mu.Lock()
	r.connection = connection
	r.listener = listener
	r.mu.Unlock()
}

func (r *runtimeTunnel) connectionValue() port.SSHConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connection
}

func (r *runtimeTunnel) closeAttempt() {
	r.attemptMu.Lock()
	defer r.attemptMu.Unlock()
	r.mu.Lock()
	listener := r.listener
	connection := r.connection
	r.listener = nil
	r.connection = nil
	r.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	if connection != nil {
		_ = connection.Close()
	}
	r.accepting.Wait()
	r.mu.Lock()
	connections := make([]net.Conn, 0, len(r.connections))
	for item := range r.connections {
		connections = append(connections, item)
	}
	r.mu.Unlock()
	for _, item := range connections {
		_ = item.Close()
	}
	r.wait.Wait()
}

func configIndex(configs []tunneldomain.Config, id string) int {
	for index, item := range configs {
		if item.ID == id {
			return index
		}
	}
	return -1
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate tunnel id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func errorMessage(err error) string {
	if err == nil {
		return "tunnel stopped unexpectedly"
	}
	return err.Error()
}
