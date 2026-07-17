package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	settingsdomain "shh-h/internal/domain/settings"
	"shh-h/internal/port"
)

const (
	keepAliveRequest  = "keepalive@openssh.com"
	groupCloseTimeout = 2 * time.Second
)

var (
	ErrPoolClosed  = errors.New("ssh connection pool is closed")
	ErrLeaseClosed = errors.New("ssh connection lease is closed")
)

// Dialer establishes one authenticated and host-key-verified SSH client.
type Dialer interface {
	DialSSH(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (*ssh.Client, error)
}

// ConnectionSettingsSource provides the validated policy captured by each new
// authenticated connection.
type ConnectionSettingsSource interface {
	ConnectionSettings() settingsdomain.Connection
}

type managedClient interface {
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
	Dial(network, address string) (net.Conn, error)
	Listen(network, address string) (net.Listener, error)
	Wait() error
	Close() error
	SSHClient() *ssh.Client
}

type clientDialer func(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (managedClient, error)

// Pool owns authenticated SSH clients and lends independent references to
// feature adapters while their work overlaps.
type Pool struct {
	mu           sync.Mutex
	dial         clientDialer
	settings     ConnectionSettingsSource
	groups       map[groupKey]*connectionGroup
	closed       bool
	shutdownDone chan struct{}
	shutdownErr  error
}

type groupKey struct {
	profileID      string
	host           string
	port           int
	username       string
	authentication string
	identityFile   string
}

type connectionGroup struct {
	key        groupKey
	ctx        context.Context
	cancel     context.CancelFunc
	ready      chan struct{}
	done       chan struct{}
	client     managedClient
	connecting bool
	finished   bool
	refs       int
	dialErr    error
	waitErr    error
}

// Lease is one feature's reference to a shared authenticated SSH client.
type Lease struct {
	pool      *Pool
	group     *connectionGroup
	closed    chan struct{}
	closeOnce sync.Once
	closeErr  error
}

type realClient struct {
	*ssh.Client
}

// NewPool creates a connection pool without opening a network connection.
func NewPool(dialer Dialer, settings ConnectionSettingsSource) *Pool {
	var dial clientDialer
	if dialer != nil {
		dial = func(ctx context.Context, spec port.SSHTerminalSpec, policy settingsdomain.Connection) (managedClient, error) {
			client, err := dialer.DialSSH(ctx, spec, policy)
			if err != nil {
				return nil, err
			}
			return &realClient{Client: client}, nil
		}
	}
	return newPool(dial, settings)
}

func newPool(dial clientDialer, settings ConnectionSettingsSource) *Pool {
	return &Pool{
		dial: dial, settings: settings, groups: make(map[groupKey]*connectionGroup),
		shutdownDone: make(chan struct{}),
	}
}

// Acquire returns a lease for the effective SSH profile, serializing the first
// authenticated dial when several features open it concurrently.
func (p *Pool) Acquire(ctx context.Context, spec port.SSHTerminalSpec) (*Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := keyFor(spec)
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, ErrPoolClosed
		}
		if group := p.groups[key]; group != nil {
			if group.connecting {
				ready := group.ready
				p.mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-ready:
					if group.dialErr != nil {
						return nil, group.dialErr
					}
					continue
				}
			}
			if !group.finished {
				group.refs++
				lease := newLease(p, group)
				p.mu.Unlock()
				return lease, nil
			}
			delete(p.groups, key)
		}

		groupContext, cancel := context.WithCancel(context.Background())
		group := &connectionGroup{
			key: key, ctx: groupContext, cancel: cancel,
			ready: make(chan struct{}), done: make(chan struct{}), connecting: true,
		}
		p.groups[key] = group
		p.mu.Unlock()
		return p.connect(ctx, spec, group)
	}
}

func (p *Pool) OpenSSHConnection(ctx context.Context, spec port.SSHTerminalSpec) (port.SSHConnection, error) {
	return p.Acquire(ctx, spec)
}

func (p *Pool) connect(ctx context.Context, spec port.SSHTerminalSpec, group *connectionGroup) (*Lease, error) {
	if p.dial == nil {
		return nil, p.failDial(group, errors.New("ssh client dialer is unavailable"))
	}

	policy := p.connectionSettings()
	dialContext, cancelDial := context.WithCancel(ctx)
	stopGroupCancellation := context.AfterFunc(group.ctx, cancelDial)
	client, err := p.dial(dialContext, spec, policy)
	stopGroupCancellation()
	cancelDial()
	if err == nil {
		err = ctx.Err()
	}

	p.mu.Lock()
	stale := p.closed || p.groups[group.key] != group || group.ctx.Err() != nil
	if err == nil && stale {
		err = ErrPoolClosed
	}
	if err != nil {
		if p.groups[group.key] == group {
			delete(p.groups, group.key)
		}
		group.connecting = false
		group.finished = true
		group.dialErr = err
		group.waitErr = err
		close(group.ready)
		close(group.done)
		p.mu.Unlock()
		group.cancel()
		if client != nil {
			_ = client.Close()
		}
		return nil, err
	}

	group.client = client
	group.connecting = false
	group.refs = 1
	close(group.ready)
	lease := newLease(p, group)
	p.mu.Unlock()

	go maintainConnection(group.ctx, client, policy)
	go p.watch(group)
	return lease, nil
}

func (p *Pool) failDial(group *connectionGroup, err error) error {
	p.mu.Lock()
	if p.groups[group.key] == group {
		delete(p.groups, group.key)
	}
	group.connecting = false
	group.finished = true
	group.dialErr = err
	group.waitErr = err
	close(group.ready)
	close(group.done)
	p.mu.Unlock()
	group.cancel()
	return err
}

func (p *Pool) watch(group *connectionGroup) {
	err := group.client.Wait()
	group.cancel()
	p.mu.Lock()
	if !group.finished {
		group.finished = true
		group.waitErr = err
		close(group.done)
	}
	if p.groups[group.key] == group {
		delete(p.groups, group.key)
	}
	p.mu.Unlock()
}

func (p *Pool) release(group *connectionGroup) error {
	p.mu.Lock()
	if group.refs > 0 {
		group.refs--
	}
	closeGroup := group.refs == 0 && !group.finished
	if closeGroup && p.groups[group.key] == group {
		delete(p.groups, group.key)
	}
	client := group.client
	p.mu.Unlock()
	if !closeGroup || client == nil {
		return nil
	}

	group.cancel()
	closeErr := client.Close()
	select {
	case <-group.done:
		return closeErr
	case <-time.After(groupCloseTimeout):
		return errors.Join(closeErr, errors.New("timed out waiting for SSH connection to close"))
	}
}

// Shutdown cancels pending dials and closes every authenticated client that
// remains after the feature managers have released their leases.
func (p *Pool) Shutdown() error {
	p.mu.Lock()
	if p.closed {
		done := p.shutdownDone
		p.mu.Unlock()
		<-done
		p.mu.Lock()
		err := p.shutdownErr
		p.mu.Unlock()
		return err
	}
	p.closed = true
	groups := make([]*connectionGroup, 0, len(p.groups))
	for key, group := range p.groups {
		delete(p.groups, key)
		groups = append(groups, group)
	}
	p.mu.Unlock()

	var failures []error
	for _, group := range groups {
		group.cancel()
		if group.client != nil {
			failures = append(failures, group.client.Close())
		}
	}
	deadline := time.Now().Add(groupCloseTimeout)
waitGroups:
	for _, group := range groups {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			failures = append(failures, errors.New("timed out waiting for SSH connections to close"))
			break
		}
		timer := time.NewTimer(remaining)
		select {
		case <-group.done:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			failures = append(failures, errors.New("timed out waiting for SSH connections to close"))
			break waitGroups
		}
	}
	err := errors.Join(failures...)
	p.mu.Lock()
	p.shutdownErr = err
	close(p.shutdownDone)
	p.mu.Unlock()
	return err
}

func (p *Pool) connectionSettings() settingsdomain.Connection {
	if p.settings == nil {
		return settingsdomain.Defaults().Connection
	}
	return p.settings.ConnectionSettings()
}

func newLease(pool *Pool, group *connectionGroup) *Lease {
	return &Lease{pool: pool, group: group, closed: make(chan struct{})}
}

func (l *Lease) Client() *ssh.Client {
	client, err := l.activeClient()
	if err != nil {
		return nil
	}
	return client.SSHClient()
}

func (l *Lease) Dial(network, address string) (net.Conn, error) {
	client, err := l.activeClient()
	if err != nil {
		return nil, err
	}
	return client.Dial(network, address)
}

func (l *Lease) Listen(network, address string) (net.Listener, error) {
	client, err := l.activeClient()
	if err != nil {
		return nil, err
	}
	return client.Listen(network, address)
}

func (l *Lease) Wait() error {
	if l == nil || l.group == nil {
		return ErrLeaseClosed
	}
	select {
	case <-l.closed:
		return ErrLeaseClosed
	case <-l.group.done:
		if l.group.waitErr == nil {
			return errors.New("ssh connection closed")
		}
		return l.group.waitErr
	}
}

func (l *Lease) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		close(l.closed)
		if l.pool != nil && l.group != nil {
			l.closeErr = l.pool.release(l.group)
		}
	})
	return l.closeErr
}

func (l *Lease) activeClient() (managedClient, error) {
	if l == nil || l.group == nil || l.group.client == nil {
		return nil, ErrLeaseClosed
	}
	select {
	case <-l.closed:
		return nil, ErrLeaseClosed
	default:
	}
	select {
	case <-l.group.done:
		if l.group.waitErr != nil {
			return nil, fmt.Errorf("ssh connection closed: %w", l.group.waitErr)
		}
		return nil, errors.New("ssh connection closed")
	default:
		return l.group.client, nil
	}
}

func (c *realClient) SSHClient() *ssh.Client {
	return c.Client
}

func keyFor(spec port.SSHTerminalSpec) groupKey {
	host := strings.Trim(strings.TrimSpace(spec.Host), "[]")
	return groupKey{
		profileID: strings.TrimSpace(spec.ProfileID), host: strings.ToLower(host), port: spec.Port,
		username: strings.TrimSpace(spec.Username), authentication: string(spec.Authentication),
		identityFile: strings.TrimSpace(spec.IdentityFile),
	}
}

func maintainConnection(ctx context.Context, client managedClient, settings settingsdomain.Connection) {
	maintainConnectionAtInterval(
		ctx,
		client,
		settings.KeepAliveEnabled,
		time.Duration(settings.KeepAliveIntervalSeconds)*time.Second,
		settings.KeepAliveMaxFailures,
	)
}

func maintainConnectionAtInterval(ctx context.Context, client interface {
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
	Close() error
}, enabled bool, interval time.Duration, maxFailures int) {
	if !enabled {
		<-ctx.Done()
		return
	}
	if interval <= 0 || maxFailures <= 0 {
		_ = client.Close()
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	results := make(chan error, maxFailures)
	pending := 0

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-results:
			if pending > 0 {
				pending--
			}
			if err != nil {
				_ = client.Close()
				return
			}
		case <-ticker.C:
			if pending >= maxFailures {
				_ = client.Close()
				return
			}
			pending++
			go func() {
				_, _, err := client.SendRequest(keepAliveRequest, true, nil)
				select {
				case results <- err:
				case <-ctx.Done():
				}
			}()
		}
	}
}

var _ port.SSHConnectionFactory = (*Pool)(nil)
var _ port.SSHConnection = (*Lease)(nil)
