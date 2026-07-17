package sshterminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"shh-h/internal/adapter/sshclient"
	"shh-h/internal/apperror"
	"shh-h/internal/domain/profile"
	settingsdomain "shh-h/internal/domain/settings"
	"shh-h/internal/domain/sshconnection"
	"shh-h/internal/port"
)

const (
	maximumKeySize  = 4 * 1024 * 1024
	defaultTerminal = "xterm-256color"
)

var (
	ErrCredentialsRequired = apperror.New(apperror.CodeAuthenticationRequired, "SSH credentials are required.")
	ErrPassphraseRequired  = apperror.New(apperror.CodeAuthenticationRequired, "Private key passphrase is required.")
	ErrAgentUnavailable    = apperror.New(apperror.CodeUnavailable, "SSH agent is unavailable.")
)

type hostKeyVerifier interface {
	HostKeyCallback(host string, port int) ssh.HostKeyCallback
}

type clientAcquirer interface {
	Acquire(context.Context, port.SSHTerminalSpec) (*sshclient.Lease, error)
}

type Factory struct {
	clients clientAcquirer
}

type Dialer struct {
	trust hostKeyVerifier
}

func NewFactory(clients clientAcquirer) *Factory {
	return &Factory{clients: clients}
}

func NewDialer(trust hostKeyVerifier) *Dialer {
	return &Dialer{trust: trust}
}

func InspectAuthentication(selected profile.Profile) (sshconnection.AuthenticationInfo, error) {
	switch selected.Authentication {
	case profile.AuthenticationPassword:
		return sshconnection.AuthenticationInfo{Secret: sshconnection.SecretPassword}, nil
	case profile.AuthenticationAgent:
		if agentSocket() == "" {
			return sshconnection.AuthenticationInfo{}, ErrAgentUnavailable
		}
		return sshconnection.AuthenticationInfo{Secret: sshconnection.SecretNone}, nil
	case profile.AuthenticationKey:
		path, err := expandIdentityPath(selected.IdentityFile)
		if err != nil {
			return sshconnection.AuthenticationInfo{}, err
		}
		secret, err := inspectPrivateKey(path)
		return sshconnection.AuthenticationInfo{Secret: secret, IdentityFile: path}, err
	case profile.AuthenticationAuto:
		if agentSocket() != "" {
			return sshconnection.AuthenticationInfo{Secret: sshconnection.SecretNone}, nil
		}
		for _, candidate := range identityCandidates(selected.IdentityFile) {
			secret, err := inspectPrivateKey(candidate)
			if err == nil && secret == sshconnection.SecretNone {
				return sshconnection.AuthenticationInfo{Secret: sshconnection.SecretNone, IdentityFile: candidate}, nil
			}
		}
		return sshconnection.AuthenticationInfo{Secret: sshconnection.SecretPassword}, nil
	default:
		return sshconnection.AuthenticationInfo{}, fmt.Errorf("unsupported ssh authentication %q", selected.Authentication)
	}
}

func (f *Factory) InspectAuthentication(selected profile.Profile) (sshconnection.AuthenticationInfo, error) {
	return InspectAuthentication(selected)
}

func (d *Dialer) InspectAuthentication(selected profile.Profile) (sshconnection.AuthenticationInfo, error) {
	return InspectAuthentication(selected)
}

func (f *Factory) OpenSSH(ctx context.Context, spec port.SSHTerminalSpec) (port.TerminalTransport, error) {
	if f.clients == nil {
		return nil, errors.New("SSH client pool is unavailable")
	}
	lease, err := f.clients.Acquire(ctx, spec)
	if err != nil {
		return nil, err
	}
	client := lease.Client()
	if client == nil {
		_ = lease.Close()
		return nil, errors.New("SSH client is unavailable")
	}
	session, err := client.NewSession()
	if err != nil {
		_ = lease.Close()
		return nil, fmt.Errorf("open ssh session: %w", err)
	}

	reader, outputWriter := io.Pipe()
	combinedOutput := &lockedWriter{writer: outputWriter}
	session.Stdout = combinedOutput
	session.Stderr = combinedOutput
	input, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = lease.Close()
		_ = reader.Close()
		_ = outputWriter.Close()
		return nil, fmt.Errorf("open ssh terminal input: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty(defaultTerminal, int(spec.Rows), int(spec.Columns), modes); err != nil {
		_ = session.Close()
		_ = lease.Close()
		_ = reader.Close()
		_ = outputWriter.Close()
		return nil, fmt.Errorf("request ssh terminal: %w", err)
	}
	if err := session.Shell(); err != nil {
		_ = session.Close()
		_ = lease.Close()
		_ = reader.Close()
		_ = outputWriter.Close()
		return nil, fmt.Errorf("start ssh shell: %w", err)
	}

	result := &transport{
		lease: lease, session: session, input: input,
		output: reader, outputWriter: outputWriter,
	}
	go func() {
		<-ctx.Done()
		_ = result.Close()
	}()
	return result, nil
}

func (d *Dialer) DialSSH(ctx context.Context, spec port.SSHTerminalSpec, connectionSettings settingsdomain.Connection) (*ssh.Client, error) {
	if d.trust == nil {
		return nil, errors.New("ssh host-key verifier is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	username := strings.TrimSpace(spec.Username)
	if username == "" {
		username = currentUsername()
	}
	authMethods, authClosers, err := buildAuthMethods(ctx, spec)
	if err != nil {
		return nil, err
	}
	defer closeAll(authClosers)

	connectTimeout := time.Duration(connectionSettings.ConnectTimeoutSeconds) * time.Second
	address := net.JoinHostPort(strings.Trim(spec.Host, "[]"), fmt.Sprintf("%d", spec.Port))
	dialContext, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	dialer := &net.Dialer{Timeout: connectTimeout, KeepAlive: -1}
	if connectionSettings.KeepAliveEnabled {
		dialer.KeepAlive = time.Duration(connectionSettings.KeepAliveIntervalSeconds) * time.Second
	}
	rawConnection, err := dialer.DialContext(dialContext, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial ssh host %s: %w", address, err)
	}

	config := &ssh.ClientConfig{
		User: username, Auth: authMethods,
		HostKeyCallback: d.trust.HostKeyCallback(spec.Host, spec.Port),
		Timeout:         connectTimeout,
	}
	connection, channels, requests, err := establishClientConnection(dialContext, rawConnection, address, config)
	if err != nil {
		_ = rawConnection.Close()
		return nil, classifyHandshakeError(err)
	}
	client := ssh.NewClient(connection, channels, requests)
	return client, nil
}

type transport struct {
	lease        *sshclient.Lease
	session      *ssh.Session
	input        io.WriteCloser
	output       *io.PipeReader
	outputWriter *io.PipeWriter
	writeMu      sync.Mutex
	requestMu    sync.Mutex
	closeOnce    sync.Once
	closeErr     error
}

func (t *transport) Read(data []byte) (int, error) {
	return t.output.Read(data)
}

func (t *transport) Write(data []byte) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.input.Write(data)
}

func (t *transport) Resize(ctx context.Context, columns, rows uint16) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.requestMu.Lock()
	defer t.requestMu.Unlock()
	if err := t.session.WindowChange(int(rows), int(columns)); err != nil {
		return fmt.Errorf("resize ssh terminal: %w", err)
	}
	return nil
}

func (t *transport) Signal(ctx context.Context, signal port.TerminalSignal) error {
	if err := ctx.Err(); err != nil && signal != port.SignalKill {
		return err
	}
	t.requestMu.Lock()
	defer t.requestMu.Unlock()
	if signal == port.SignalKill {
		return t.Close()
	}
	var remoteSignal ssh.Signal
	switch signal {
	case port.SignalHangup:
		remoteSignal = ssh.SIGHUP
	case port.SignalInterrupt:
		remoteSignal = ssh.SIGINT
	case port.SignalTerminate:
		remoteSignal = ssh.SIGTERM
	default:
		return fmt.Errorf("unsupported ssh terminal signal %q", signal)
	}
	if err := t.session.Signal(remoteSignal); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("signal ssh terminal: %w", err)
	}
	return nil
}

func (t *transport) Wait() (port.ExitStatus, error) {
	err := t.session.Wait()
	_ = t.outputWriter.Close()
	if err == nil {
		return port.ExitStatus{Code: 0}, nil
	}
	var exitError *ssh.ExitError
	if errors.As(err, &exitError) {
		return port.ExitStatus{Code: exitError.ExitStatus(), Signal: exitError.Signal()}, nil
	}
	return port.ExitStatus{}, fmt.Errorf("wait for ssh shell: %w", err)
}

func (t *transport) Close() error {
	t.closeOnce.Do(func() {
		var failures []error
		failures = append(failures, meaningfulCloseError(t.input.Close()))
		failures = append(failures, meaningfulCloseError(t.session.Close()))
		failures = append(failures, meaningfulCloseError(t.lease.Close()))
		failures = append(failures, meaningfulCloseError(t.outputWriter.Close()))
		failures = append(failures, meaningfulCloseError(t.output.Close()))
		t.closeErr = errors.Join(failures...)
	})
	return t.closeErr
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

type handshakeResult struct {
	connection ssh.Conn
	channels   <-chan ssh.NewChannel
	requests   <-chan *ssh.Request
	err        error
}

func establishClientConnection(ctx context.Context, raw net.Conn, address string, config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	done := make(chan handshakeResult, 1)
	go func() {
		connection, channels, requests, err := ssh.NewClientConn(raw, address, config)
		done <- handshakeResult{connection: connection, channels: channels, requests: requests, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = raw.Close()
		result := <-done
		if result.connection != nil {
			_ = result.connection.Close()
		}
		return nil, nil, nil, ctx.Err()
	case result := <-done:
		return result.connection, result.channels, result.requests, result.err
	}
}

func buildAuthMethods(ctx context.Context, spec port.SSHTerminalSpec) ([]ssh.AuthMethod, []io.Closer, error) {
	methods := make([]ssh.AuthMethod, 0, 4)
	closers := make([]io.Closer, 0, 1)

	useAgent := spec.Authentication == profile.AuthenticationAuto || spec.Authentication == profile.AuthenticationAgent
	if useAgent {
		connection, err := connectAgent(ctx)
		if err == nil {
			closers = append(closers, connection)
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(connection).Signers))
		} else if spec.Authentication == profile.AuthenticationAgent {
			return nil, nil, err
		}
	}

	useKeys := spec.Authentication == profile.AuthenticationAuto || spec.Authentication == profile.AuthenticationKey
	if useKeys {
		candidates := identityCandidates(spec.IdentityFile)
		if spec.Authentication == profile.AuthenticationKey {
			path, err := expandIdentityPath(spec.IdentityFile)
			if err != nil {
				closeAll(closers)
				return nil, nil, err
			}
			candidates = []string{path}
		}
		for _, candidate := range candidates {
			signer, err := parsePrivateKey(candidate, spec.Credentials.Passphrase)
			if err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
				continue
			}
			if errors.Is(err, os.ErrNotExist) && spec.Authentication == profile.AuthenticationAuto {
				continue
			}
			if errors.Is(err, ErrPassphraseRequired) && spec.Authentication == profile.AuthenticationAuto {
				continue
			}
			closeAll(closers)
			return nil, nil, err
		}
	}

	if len(spec.Credentials.Password) > 0 {
		password := string(spec.Credentials.Password)
		methods = append(methods,
			ssh.Password(password),
			ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for index := range answers {
					answers[index] = password
				}
				return answers, nil
			}),
		)
	}

	if len(methods) == 0 {
		closeAll(closers)
		return nil, nil, ErrCredentialsRequired
	}
	return methods, closers, nil
}

func inspectPrivateKey(path string) (sshconnection.SecretRequirement, error) {
	data, err := readKeyFile(path)
	if err != nil {
		return "", err
	}
	_, err = ssh.ParsePrivateKey(data)
	clear(data)
	if err == nil {
		return sshconnection.SecretNone, nil
	}
	var passphraseMissing *ssh.PassphraseMissingError
	if errors.As(err, &passphraseMissing) {
		return sshconnection.SecretPassphrase, nil
	}
	return "", fmt.Errorf("parse private key %q: %w", path, err)
}

func parsePrivateKey(path string, passphrase []byte) (ssh.Signer, error) {
	data, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	signer, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return signer, nil
	}
	var passphraseMissing *ssh.PassphraseMissingError
	if !errors.As(err, &passphraseMissing) {
		return nil, fmt.Errorf("parse private key %q: %w", path, err)
	}
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("%w for %s", ErrPassphraseRequired, path)
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt private key %q: %w", path, err)
	}
	return signer, nil
}

func readKeyFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open private key %q: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximumKeySize+1))
	if err != nil {
		return nil, fmt.Errorf("read private key %q: %w", path, err)
	}
	if len(data) > maximumKeySize {
		clear(data)
		return nil, fmt.Errorf("private key %q exceeds %d bytes", path, maximumKeySize)
	}
	return data, nil
}

func identityCandidates(configured string) []string {
	if strings.TrimSpace(configured) != "" {
		path, err := expandIdentityPath(configured)
		if err == nil {
			return []string{path}
		}
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}
}

func expandIdentityPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("identity file is required")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve identity file: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return filepath.Clean(path), nil
}

func connectAgent(ctx context.Context) (net.Conn, error) {
	socket := agentSocket()
	if socket == "" {
		return nil, ErrAgentUnavailable
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, fmt.Errorf("connect ssh agent: %w", err)
	}
	return connection, nil
}

func agentSocket() string {
	return strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
}

func currentUsername() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	if username := strings.TrimSpace(os.Getenv("USER")); username != "" {
		return username
	}
	return "unknown"
}

func classifyHandshakeError(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unable to authenticate") || strings.Contains(message, "no supported methods remain") {
		return fmt.Errorf("ssh authentication failed: %w", err)
	}
	return fmt.Errorf("establish ssh connection: %w", err)
}

func meaningfulCloseError(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func closeAll(closers []io.Closer) {
	for _, closer := range closers {
		_ = closer.Close()
	}
}

var _ port.SSHTerminalFactory = (*Factory)(nil)
var _ port.TerminalTransport = (*transport)(nil)
