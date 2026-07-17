package sshtrust

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"shh-h/internal/domain/profile"
	settingsdomain "shh-h/internal/domain/settings"
	"shh-h/internal/domain/sshconnection"
)

const (
	challengeTimeout = 2 * time.Minute
)

type connectionSettingsSource interface {
	ConnectionSettings() settingsdomain.Connection
}

type pendingKey struct {
	leaseID   string
	address   string
	key       ssh.PublicKey
	expiresAt time.Time
}

type Store struct {
	mu              sync.Mutex
	applicationPath string
	userPath        string
	settings        connectionSettingsSource
	sessionKeys     map[string][]byte
	pending         map[string]pendingKey
}

var errHostKeyCaptured = errors.New("ssh host key captured")

func New(appID string, settings connectionSettingsSource) (*Store, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve application config directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return newStore(
		filepath.Join(configDirectory, appID, "known_hosts"),
		filepath.Join(home, ".ssh", "known_hosts"),
		settings,
	), nil
}

func NewAt(applicationPath, userPath string) *Store {
	return newStore(applicationPath, userPath, nil)
}

func newStore(applicationPath, userPath string, settings connectionSettingsSource) *Store {
	return &Store{
		applicationPath: applicationPath,
		userPath:        userPath,
		settings:        settings,
		sessionKeys:     make(map[string][]byte),
		pending:         make(map[string]pendingKey),
	}
}

func (s *Store) Probe(ctx context.Context, leaseID string, selected profile.Profile) (sshconnection.HostKeyInfo, error) {
	if strings.TrimSpace(leaseID) == "" {
		return sshconnection.HostKeyInfo{}, errors.New("frontend lease is required")
	}
	if selected.Protocol != profile.ProtocolSSH {
		return sshconnection.HostKeyInfo{}, errors.New("host key probing requires an ssh profile")
	}
	address := profileAddress(selected.Host, selected.Port)
	timeout := time.Duration(s.connectionSettings().ConnectTimeoutSeconds) * time.Second
	key, err := probeHostKey(ctx, address, selected.Username, timeout)
	if err != nil {
		return sshconnection.HostKeyInfo{}, err
	}

	status, err := s.status(address, key)
	if err != nil {
		return sshconnection.HostKeyInfo{}, err
	}
	result := sshconnection.HostKeyInfo{
		Status: status, Host: selected.Host, Address: address,
		Algorithm: key.Type(), Fingerprint: ssh.FingerprintSHA256(key),
	}
	if status != sshconnection.HostKeyUnknown {
		return result, nil
	}

	challenge, err := randomID()
	if err != nil {
		return sshconnection.HostKeyInfo{}, err
	}
	s.mu.Lock()
	s.removeExpiredLocked(time.Now())
	s.pending[challenge] = pendingKey{
		leaseID: leaseID, address: address, key: key, expiresAt: time.Now().Add(challengeTimeout),
	}
	s.mu.Unlock()
	result.ChallengeID = challenge
	return result, nil
}

func (s *Store) connectionSettings() settingsdomain.Connection {
	if s.settings == nil {
		return settingsdomain.Defaults().Connection
	}
	return s.settings.ConnectionSettings()
}

func (s *Store) Trust(leaseID, challengeID string, permanent bool) error {
	now := time.Now()
	s.mu.Lock()
	s.removeExpiredLocked(now)
	pending, exists := s.pending[challengeID]
	if !exists || pending.leaseID != leaseID {
		s.mu.Unlock()
		return errors.New("host key trust challenge is missing, stale, or belongs to another frontend")
	}
	delete(s.pending, challengeID)
	s.mu.Unlock()

	status, err := s.status(pending.address, pending.key)
	if err != nil {
		return err
	}
	if status == sshconnection.HostKeyChanged {
		return errors.New("host key changed while trust confirmation was open")
	}
	if status == sshconnection.HostKeyKnown {
		return nil
	}

	if permanent {
		if err := s.appendKnownHost(pending.address, pending.key); err != nil {
			return err
		}
		return nil
	}

	s.mu.Lock()
	s.sessionKeys[pending.address] = append([]byte(nil), pending.key.Marshal()...)
	s.mu.Unlock()
	return nil
}

func (s *Store) HostKeyCallback(host string, port int) ssh.HostKeyCallback {
	address := profileAddress(host, port)
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		status, err := s.status(address, key)
		if err != nil {
			return err
		}
		switch status {
		case sshconnection.HostKeyKnown:
			return nil
		case sshconnection.HostKeyChanged:
			return fmt.Errorf("ssh host key for %s has changed", address)
		default:
			return fmt.Errorf("ssh host key for %s is not trusted", address)
		}
	}
}

func (s *Store) status(address string, key ssh.PublicKey) (sshconnection.HostKeyStatus, error) {
	s.mu.Lock()
	sessionKey := append([]byte(nil), s.sessionKeys[address]...)
	s.mu.Unlock()
	if len(sessionKey) > 0 {
		if string(sessionKey) == string(key.Marshal()) {
			return sshconnection.HostKeyKnown, nil
		}
		return sshconnection.HostKeyChanged, nil
	}

	paths := make([]string, 0, 2)
	for _, path := range []string{s.userPath, s.applicationPath} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			paths = append(paths, path)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect known hosts %q: %w", path, err)
		}
	}
	if len(paths) == 0 {
		return sshconnection.HostKeyUnknown, nil
	}

	callback, err := knownhosts.New(paths...)
	if err != nil {
		return "", fmt.Errorf("load known hosts: %w", err)
	}
	err = callback(address, remoteAddress(address), key)
	if err == nil {
		return sshconnection.HostKeyKnown, nil
	}
	var keyError *knownhosts.KeyError
	if errors.As(err, &keyError) {
		if len(keyError.Want) == 0 {
			return sshconnection.HostKeyUnknown, nil
		}
		return sshconnection.HostKeyChanged, nil
	}
	return "", fmt.Errorf("verify ssh host key: %w", err)
}

func (s *Store) appendKnownHost(address string, key ssh.PublicKey) error {
	directory := filepath.Dir(s.applicationPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create known-hosts directory: %w", err)
	}
	file, err := os.OpenFile(s.applicationPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open application known hosts: %w", err)
	}
	line := knownhosts.Line([]string{knownhosts.Normalize(address)}, key) + "\n"
	if _, err := file.WriteString(line); err != nil {
		_ = file.Close()
		return fmt.Errorf("write application known hosts: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync application known hosts: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close application known hosts: %w", err)
	}
	if err := os.Chmod(s.applicationPath, 0o600); err != nil {
		return fmt.Errorf("protect application known hosts: %w", err)
	}
	return nil
}

func (s *Store) removeExpiredLocked(now time.Time) {
	for id, pending := range s.pending {
		if !now.Before(pending.expiresAt) {
			delete(s.pending, id)
		}
	}
}

func probeHostKey(ctx context.Context, address, username string, timeout time.Duration) (ssh.PublicKey, error) {
	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(probeContext, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial ssh host %s: %w", address, err)
	}
	defer raw.Close()

	if strings.TrimSpace(username) == "" {
		username = "shh-h-probe"
	}
	var captured ssh.PublicKey
	config := &ssh.ClientConfig{
		User: username,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			captured = key
			return errHostKeyCaptured
		},
		Timeout: timeout,
	}

	done := make(chan error, 1)
	go func() {
		_, _, _, handshakeErr := ssh.NewClientConn(raw, address, config)
		done <- handshakeErr
	}()
	select {
	case <-probeContext.Done():
		_ = raw.Close()
		<-done
		return nil, fmt.Errorf("probe ssh host key: %w", probeContext.Err())
	case handshakeErr := <-done:
		if captured == nil {
			return nil, fmt.Errorf("probe ssh host key: %w", handshakeErr)
		}
		return captured, nil
	}
}

func profileAddress(host string, port int) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func remoteAddress(address string) net.Addr {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return &net.TCPAddr{}
	}
	port, _ := strconv.Atoi(portText)
	return &net.TCPAddr{IP: net.ParseIP(host), Port: port}
}

func randomID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate host-key challenge: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
