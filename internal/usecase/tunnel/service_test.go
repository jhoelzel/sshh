package tunnel

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"shh-h/internal/domain/profile"
	tunneldomain "shh-h/internal/domain/tunnel"
	"shh-h/internal/port"
)

func TestLocalTunnelRoundTripAndStop(t *testing.T) {
	target := startEchoServer(t)
	defer target.Close()
	service := newTestService(t, tunneldomain.Config{
		ID: "local", Name: "Local", ProfileID: "ssh", Kind: tunneldomain.KindLocal,
		BindAddress: "127.0.0.1", DestinationHost: "127.0.0.1", DestinationPort: portOf(t, target.Addr()),
	})
	snapshot, err := service.Start(context.Background(), "lease", "local", port.SSHCredentials{})
	if err != nil {
		t.Fatalf("start local tunnel: %v", err)
	}
	assertRoundTrip(t, snapshot.BoundAddress)
	held, err := net.DialTimeout("tcp", snapshot.BoundAddress, time.Second)
	if err != nil {
		t.Fatalf("open active relay: %v", err)
	}
	if err := service.Stop("lease", "local"); err != nil {
		t.Fatalf("stop local tunnel: %v", err)
	}
	_ = held.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := held.Read(make([]byte, 1)); err == nil {
		t.Fatal("active relay remained open after stop")
	}
	_ = held.Close()
	if state := service.Snapshots("lease")[0].State; state != tunneldomain.StateStopped {
		t.Fatalf("unexpected stopped state: %s", state)
	}
	connection, err := net.DialTimeout("tcp", snapshot.BoundAddress, 100*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatal("listener still accepted connections after stop")
	}
}

func TestRemoteTunnelRoundTrip(t *testing.T) {
	target := startEchoServer(t)
	defer target.Close()
	service := newTestService(t, tunneldomain.Config{
		ID: "remote", Name: "Remote", ProfileID: "ssh", Kind: tunneldomain.KindRemote,
		BindAddress: "127.0.0.1", DestinationHost: "127.0.0.1", DestinationPort: portOf(t, target.Addr()),
	})
	snapshot, err := service.Start(context.Background(), "lease", "remote", port.SSHCredentials{})
	if err != nil {
		t.Fatalf("start remote tunnel: %v", err)
	}
	defer service.Stop("lease", "remote")
	assertRoundTrip(t, snapshot.BoundAddress)
}

func TestDynamicSOCKS5TunnelRoundTrip(t *testing.T) {
	target := startEchoServer(t)
	defer target.Close()
	service := newTestService(t, tunneldomain.Config{
		ID: "socks", Name: "SOCKS", ProfileID: "ssh", Kind: tunneldomain.KindDynamic,
		BindAddress: "127.0.0.1",
	})
	snapshot, err := service.Start(context.Background(), "lease", "socks", port.SSHCredentials{})
	if err != nil {
		t.Fatalf("start dynamic tunnel: %v", err)
	}
	defer service.Stop("lease", "socks")
	connection, err := net.Dial("tcp", snapshot.BoundAddress)
	if err != nil {
		t.Fatalf("dial SOCKS listener: %v", err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("write SOCKS greeting: %v", err)
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(connection, response); err != nil || response[1] != 0x00 {
		t.Fatalf("read SOCKS greeting: response=%v err=%v", response, err)
	}
	targetPort := portOf(t, target.Addr())
	request := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, 0, 0}
	binary.BigEndian.PutUint16(request[len(request)-2:], uint16(targetPort))
	if _, err := connection.Write(request); err != nil {
		t.Fatalf("write SOCKS request: %v", err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(connection, reply); err != nil || reply[1] != 0x00 {
		t.Fatalf("read SOCKS reply: response=%v err=%v", reply, err)
	}
	if _, err := connection.Write([]byte("through-socks")); err != nil {
		t.Fatalf("write SOCKS payload: %v", err)
	}
	payload := make([]byte, len("through-socks"))
	if _, err := io.ReadFull(connection, payload); err != nil || string(payload) != "through-socks" {
		t.Fatalf("SOCKS round trip: payload=%q err=%v", payload, err)
	}
}

func TestTunnelRejectsCollisionAndSecretReconnect(t *testing.T) {
	occupied := listenLoopback(t)
	defer occupied.Close()
	service := newTestService(t, tunneldomain.Config{
		ID: "collision", Name: "Collision", ProfileID: "ssh", Kind: tunneldomain.KindLocal,
		BindAddress: "127.0.0.1", BindPort: portOf(t, occupied.Addr()),
		DestinationHost: "127.0.0.1", DestinationPort: 1,
	})
	if _, err := service.Start(context.Background(), "lease", "collision", port.SSHCredentials{}); err == nil {
		t.Fatal("expected port collision to fail")
	}

	reconnect := newTestService(t, tunneldomain.Config{
		ID: "reconnect", Name: "Reconnect", ProfileID: "ssh", Kind: tunneldomain.KindDynamic,
		BindAddress: "127.0.0.1", Reconnect: true,
	})
	if _, err := reconnect.Start(context.Background(), "lease", "reconnect", port.SSHCredentials{Password: []byte("secret")}); err == nil {
		t.Fatal("expected secret-backed reconnect to be rejected")
	}
}

func TestTunnelReconnectsAfterInitialNetworkFailure(t *testing.T) {
	probe := listenLoopback(t)
	_ = probe.Close()
	config := tunneldomain.Config{
		ID: "retry", Name: "Retry", ProfileID: "ssh", Kind: tunneldomain.KindDynamic,
		BindAddress: "127.0.0.1", Reconnect: true,
	}.WithDefaults(time.Now().UTC())
	selected := profile.Profile{
		ID: "ssh", Name: "SSH", Protocol: profile.ProtocolSSH, Host: "example.test", Port: 22,
		Authentication: profile.AuthenticationAgent,
	}
	service, err := NewService(
		&memoryRepository{configs: []tunneldomain.Config{config}},
		fixedProfiles{selected: selected},
		&flakyFactory{},
	)
	if err != nil {
		t.Fatalf("create retry service: %v", err)
	}
	snapshot, err := service.Start(context.Background(), "lease", config.ID, port.SSHCredentials{})
	if err != nil || snapshot.State != tunneldomain.StateRetrying {
		t.Fatalf("initial retry state: snapshot=%#v err=%v", snapshot, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot = service.Snapshots("lease")[0]
		if snapshot.State == tunneldomain.StateActive {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if snapshot.State != tunneldomain.StateActive {
		t.Fatalf("tunnel did not reconnect: %#v", snapshot)
	}
	if err := service.Stop("lease", config.ID); err != nil {
		t.Fatalf("stop reconnected tunnel: %v", err)
	}
}

type memoryRepository struct{ configs []tunneldomain.Config }

func (r *memoryRepository) LoadTunnels() ([]tunneldomain.Config, error) {
	return append([]tunneldomain.Config{}, r.configs...), nil
}

func (r *memoryRepository) SaveTunnels(configs []tunneldomain.Config) error {
	r.configs = append([]tunneldomain.Config{}, configs...)
	return nil
}

type fixedProfiles struct{ selected profile.Profile }

func (f fixedProfiles) Find(id string) (profile.Profile, bool) {
	return f.selected, id == f.selected.ID
}

type directFactory struct{}

func (directFactory) OpenSSHConnection(context.Context, port.SSHTerminalSpec) (port.SSHConnection, error) {
	return newDirectConnection(), nil
}

type flakyFactory struct {
	mu       sync.Mutex
	attempts int
}

func (f *flakyFactory) OpenSSHConnection(context.Context, port.SSHTerminalSpec) (port.SSHConnection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.attempts == 1 {
		return nil, errors.New("network unavailable")
	}
	return newDirectConnection(), nil
}

type directConnection struct {
	closed chan struct{}
	once   sync.Once
}

func newDirectConnection() *directConnection { return &directConnection{closed: make(chan struct{})} }

func (c *directConnection) Dial(network, address string) (net.Conn, error) {
	return net.DialTimeout(network, address, time.Second)
}

func (c *directConnection) Listen(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}

func (c *directConnection) Wait() error {
	<-c.closed
	return errors.New("closed")
}

func (c *directConnection) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func newTestService(t *testing.T, config tunneldomain.Config) *Service {
	t.Helper()
	config = config.WithDefaults(time.Now().UTC())
	if err := config.Validate(); err != nil {
		t.Fatalf("invalid test config: %v", err)
	}
	selected := profile.Profile{
		ID: "ssh", Name: "SSH", Protocol: profile.ProtocolSSH, Host: "example.test", Port: 22,
		Authentication: profile.AuthenticationAgent,
	}
	service, err := NewService(&memoryRepository{configs: []tunneldomain.Config{config}}, fixedProfiles{selected: selected}, directFactory{})
	if err != nil {
		t.Fatalf("create tunnel service: %v", err)
	}
	return service
}

type echoServer struct {
	listener net.Listener
	done     chan struct{}
}

func startEchoServer(t *testing.T) *echoServer {
	t.Helper()
	listener := listenLoopback(t)
	server := &echoServer{listener: listener, done: make(chan struct{})}
	go func() {
		defer close(server.done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, _ = io.Copy(connection, connection)
			}()
		}
	}()
	return server
}

func (s *echoServer) Addr() net.Addr { return s.listener.Addr() }

func (s *echoServer) Close() {
	_ = s.listener.Close()
	select {
	case <-s.done:
	case <-time.After(time.Second):
	}
}

func listenLoopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit loopback listeners")
		}
		t.Fatalf("listen on loopback: %v", err)
	}
	return listener
}

func portOf(t *testing.T, address net.Addr) int {
	t.Helper()
	_, text, err := net.SplitHostPort(address.String())
	if err != nil {
		t.Fatalf("split address %s: %v", address, err)
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		t.Fatalf("parse port %q: %v", text, err)
	}
	return value
}

func assertRoundTrip(t *testing.T, address string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("dial tunnel %s: %v", address, err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("round-trip")); err != nil {
		t.Fatalf("write tunnel: %v", err)
	}
	payload := make([]byte, len("round-trip"))
	if _, err := io.ReadFull(connection, payload); err != nil || string(payload) != "round-trip" {
		t.Fatalf("tunnel round trip: payload=%q err=%v", payload, err)
	}
}
