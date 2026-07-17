package sshclient

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"shh-h/internal/domain/profile"
	settingsdomain "shh-h/internal/domain/settings"
	"shh-h/internal/port"
)

func TestPoolSharesClientUntilFinalLeaseCloses(t *testing.T) {
	client := newFakeClient()
	calls := 0
	pool := newPool(func(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (managedClient, error) {
		calls++
		return client, nil
	}, nil)

	firstSpec := testSpec()
	firstSpec.Credentials.Password = []byte("first")
	first, err := pool.Acquire(context.Background(), firstSpec)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	secondSpec := testSpec()
	secondSpec.Credentials.Password = []byte("second")
	secondSpec.Columns = 160
	secondSpec.Rows = 50
	second, err := pool.Acquire(context.Background(), secondSpec)
	if err != nil {
		t.Fatalf("acquire second lease: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one authenticated connection, got %d", calls)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first lease: %v", err)
	}
	if client.closeCount() != 0 {
		t.Fatal("first lease closed the shared client")
	}
	if !errors.Is(first.Wait(), ErrLeaseClosed) {
		t.Fatal("closed lease waiter did not stop independently")
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close final lease: %v", err)
	}
	if client.closeCount() != 1 {
		t.Fatalf("final lease closed client %d times", client.closeCount())
	}
}

func TestPoolSerializesConcurrentFirstDial(t *testing.T) {
	client := newFakeClient()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls int
	var callsMu sync.Mutex
	pool := newPool(func(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (managedClient, error) {
		callsMu.Lock()
		calls++
		if calls == 1 {
			close(started)
		}
		callsMu.Unlock()
		<-release
		return client, nil
	}, nil)

	type result struct {
		lease *Lease
		err   error
	}
	firstResult := make(chan result, 1)
	secondResult := make(chan result, 1)
	go func() {
		lease, err := pool.Acquire(context.Background(), testSpec())
		firstResult <- result{lease: lease, err: err}
	}()
	<-started
	go func() {
		lease, err := pool.Acquire(context.Background(), testSpec())
		secondResult <- result{lease: lease, err: err}
	}()
	close(release)

	first := <-firstResult
	second := <-secondResult
	if first.err != nil || second.err != nil {
		t.Fatalf("acquire shared leases: first=%v second=%v", first.err, second.err)
	}
	callsMu.Lock()
	actualCalls := calls
	callsMu.Unlock()
	if actualCalls != 1 {
		t.Fatalf("expected one concurrent dial, got %d", actualCalls)
	}
	_ = first.lease.Close()
	_ = second.lease.Close()
}

func TestPoolCanceledWaiterDoesNotOwnLease(t *testing.T) {
	client := newFakeClient()
	started := make(chan struct{})
	release := make(chan struct{})
	pool := newPool(func(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (managedClient, error) {
		close(started)
		<-release
		return client, nil
	}, nil)
	firstResult := make(chan *Lease, 1)
	go func() {
		lease, _ := pool.Acquire(context.Background(), testSpec())
		firstResult <- lease
	}()
	<-started

	waitContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pool.Acquire(waitContext, testSpec()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled waiter, got %v", err)
	}
	close(release)
	first := <-firstResult
	if err := first.Close(); err != nil {
		t.Fatalf("close only lease: %v", err)
	}
	if client.closeCount() != 1 {
		t.Fatalf("canceled waiter retained a reference; close count=%d", client.closeCount())
	}
}

func TestPoolRedialsAfterRemoteConnectionCloses(t *testing.T) {
	clients := []*fakeClient{newFakeClient(), newFakeClient()}
	calls := 0
	pool := newPool(func(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (managedClient, error) {
		client := clients[calls]
		calls++
		return client, nil
	}, nil)
	first, err := pool.Acquire(context.Background(), testSpec())
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	_ = clients[0].Close()
	if err := first.Wait(); !errors.Is(err, errRemoteClosed) {
		t.Fatalf("unexpected remote close error: %v", err)
	}

	second, err := pool.Acquire(context.Background(), testSpec())
	if err != nil {
		t.Fatalf("redial after close: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected a new connection, got %d dials", calls)
	}
	_ = first.Close()
	_ = second.Close()
}

func TestPoolSeparatesEffectiveConnectionKeys(t *testing.T) {
	var clients []*fakeClient
	pool := newPool(func(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (managedClient, error) {
		client := newFakeClient()
		clients = append(clients, client)
		return client, nil
	}, nil)
	first, err := pool.Acquire(context.Background(), testSpec())
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	changed := testSpec()
	changed.Host = "other.example.com"
	second, err := pool.Acquire(context.Background(), changed)
	if err != nil {
		t.Fatalf("acquire changed connection: %v", err)
	}
	if len(clients) != 2 {
		t.Fatalf("expected separate groups, got %d", len(clients))
	}
	_ = first.Close()
	_ = second.Close()
}

func TestPoolSnapshotsSettingsOncePerConnection(t *testing.T) {
	settings := &countingSettings{connection: settingsdomain.Connection{
		ConnectTimeoutSeconds: 9, KeepAliveIntervalSeconds: 45, KeepAliveMaxFailures: 2,
	}}
	client := newFakeClient()
	var captured settingsdomain.Connection
	pool := newPool(func(_ context.Context, _ port.SSHTerminalSpec, policy settingsdomain.Connection) (managedClient, error) {
		captured = policy
		return client, nil
	}, settings)
	first, err := pool.Acquire(context.Background(), testSpec())
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	second, err := pool.Acquire(context.Background(), testSpec())
	if err != nil {
		t.Fatalf("acquire second lease: %v", err)
	}
	if settings.callCount() != 1 {
		t.Fatalf("connection settings read %d times", settings.callCount())
	}
	if captured != settings.connection {
		t.Fatalf("unexpected connection policy: %#v", captured)
	}
	_ = first.Close()
	_ = second.Close()
}

func TestPoolShutdownClosesGroupsAndRejectsAcquire(t *testing.T) {
	client := newFakeClient()
	pool := newPool(func(context.Context, port.SSHTerminalSpec, settingsdomain.Connection) (managedClient, error) {
		return client, nil
	}, nil)
	lease, err := pool.Acquire(context.Background(), testSpec())
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if err := pool.Shutdown(); err != nil {
		t.Fatalf("shutdown pool: %v", err)
	}
	if client.closeCount() != 1 {
		t.Fatalf("shutdown close count=%d", client.closeCount())
	}
	if _, err := pool.Acquire(context.Background(), testSpec()); !errors.Is(err, ErrPoolClosed) {
		t.Fatalf("expected closed pool error, got %v", err)
	}
	_ = lease.Close()
}

func TestPoolShutdownCancelsInFlightDial(t *testing.T) {
	started := make(chan struct{})
	pool := newPool(func(ctx context.Context, _ port.SSHTerminalSpec, _ settingsdomain.Connection) (managedClient, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}, nil)
	result := make(chan error, 1)
	go func() {
		_, err := pool.Acquire(context.Background(), testSpec())
		result <- err
	}()
	<-started
	if err := pool.Shutdown(); err != nil {
		t.Fatalf("shutdown pool: %v", err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected dial error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("in-flight dial outlived pool shutdown")
	}
}

func TestConnectionMaintenanceClosesAfterUnansweredKeepalives(t *testing.T) {
	client := newMaintenanceClient(nil)
	done := make(chan struct{})
	go func() {
		maintainConnectionAtInterval(context.Background(), client, true, 5*time.Millisecond, 2)
		close(done)
	}()

	select {
	case <-client.closed:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("connection was not closed after unanswered keepalives")
	}
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("connection maintenance did not stop")
	}
	if requests := client.requestCount(); requests != 2 {
		t.Fatalf("unexpected keepalive request count: %d", requests)
	}
}

func TestConnectionMaintenanceResetsFailureCountOnReplies(t *testing.T) {
	client := newMaintenanceClient(func() error { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		maintainConnectionAtInterval(ctx, client, true, 5*time.Millisecond, 2)
		close(done)
	}()

	for range 3 {
		select {
		case <-client.requested:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for keepalive request")
		}
	}
	select {
	case <-client.closed:
		t.Fatal("responsive connection was closed")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("connection maintenance ignored cancellation")
	}
	if client.requestCount() < 2 {
		t.Fatalf("expected repeated keepalives, got %d", client.requestCount())
	}
}

func testSpec() port.SSHTerminalSpec {
	return port.SSHTerminalSpec{
		ProfileID: "profile-1", Host: "example.com", Port: 22, Username: "tester",
		Authentication: profile.AuthenticationPassword, Columns: 80, Rows: 24,
	}
}

var errRemoteClosed = errors.New("remote connection closed")

type fakeClient struct {
	closed    chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	closes    int
}

func newFakeClient() *fakeClient {
	return &fakeClient{closed: make(chan struct{})}
}

func (c *fakeClient) SendRequest(string, bool, []byte) (bool, []byte, error) {
	select {
	case <-c.closed:
		return false, nil, errRemoteClosed
	default:
		return true, nil, nil
	}
}

func (c *fakeClient) Dial(string, string) (net.Conn, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeClient) Listen(string, string) (net.Listener, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeClient) Wait() error {
	<-c.closed
	return errRemoteClosed
}

func (c *fakeClient) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closes++
		c.mu.Unlock()
		close(c.closed)
	})
	return nil
}

func (c *fakeClient) SSHClient() *ssh.Client {
	return nil
}

func (c *fakeClient) closeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closes
}

type maintenanceClient struct {
	mu        sync.Mutex
	requests  int
	respond   func() error
	closed    chan struct{}
	requested chan struct{}
	closeOnce sync.Once
}

type countingSettings struct {
	mu         sync.Mutex
	connection settingsdomain.Connection
	calls      int
}

func (s *countingSettings) ConnectionSettings() settingsdomain.Connection {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.connection
}

func (s *countingSettings) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func newMaintenanceClient(respond func() error) *maintenanceClient {
	return &maintenanceClient{respond: respond, closed: make(chan struct{}), requested: make(chan struct{}, 16)}
}

func (c *maintenanceClient) SendRequest(name string, wantReply bool, _ []byte) (bool, []byte, error) {
	if name != keepAliveRequest || !wantReply {
		return false, nil, errors.New("unexpected keepalive request")
	}
	c.mu.Lock()
	c.requests++
	respond := c.respond
	c.mu.Unlock()
	select {
	case c.requested <- struct{}{}:
	default:
	}
	if respond != nil {
		return false, nil, respond()
	}
	<-c.closed
	return false, nil, errors.New("closed")
}

func (c *maintenanceClient) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *maintenanceClient) requestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}
