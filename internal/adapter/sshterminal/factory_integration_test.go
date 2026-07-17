package sshterminal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"shh-h/internal/adapter/sshclient"
	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

func TestFactoryInteractiveTerminalRoundTrip(t *testing.T) {
	server := newTestSSHServer(t)
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Address())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	sshPort, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := sshclient.NewPool(NewDialer(fixedHostKey{key: server.hostKey}), nil)
	defer clients.Shutdown()
	factory := NewFactory(clients)
	terminal, err := factory.OpenSSH(ctx, port.SSHTerminalSpec{
		Host: host, Port: sshPort, Username: "tester",
		Authentication: profile.AuthenticationPassword,
		Credentials:    port.SSHCredentials{Password: []byte("secret")},
		Columns:        100, Rows: 30,
	})
	if err != nil {
		t.Fatalf("open ssh terminal: %v", err)
	}

	if _, err := terminal.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write terminal: %v", err)
	}
	output := make([]byte, len("hello\n"))
	if _, err := io.ReadFull(terminal, output); err != nil {
		t.Fatalf("read terminal: %v", err)
	}
	if string(output) != "hello\n" {
		t.Fatalf("unexpected terminal output %q", output)
	}
	if err := terminal.Resize(ctx, 120, 40); err != nil {
		t.Fatalf("resize terminal: %v", err)
	}
	select {
	case size := <-server.resize:
		if size != [2]uint32{120, 40} {
			t.Fatalf("unexpected remote size: %#v", size)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for remote resize")
	}
	if _, err := terminal.Write([]byte("exit\n")); err != nil {
		t.Fatalf("request exit: %v", err)
	}
	status, err := terminal.Wait()
	if err != nil {
		t.Fatalf("wait terminal: %v", err)
	}
	if status.Code != 7 {
		t.Fatalf("unexpected remote exit status: %#v", status)
	}
	if err := terminal.Close(); err != nil {
		t.Fatalf("close terminal: %v", err)
	}
}

func TestFactoryTerminalLeasesShareConnection(t *testing.T) {
	server := newTestSSHServer(t)
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Address())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	sshPort, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	clients := sshclient.NewPool(NewDialer(fixedHostKey{key: server.hostKey}), nil)
	defer clients.Shutdown()
	factory := NewFactory(clients)
	spec := port.SSHTerminalSpec{
		ProfileID: "shared", Host: host, Port: sshPort, Username: "tester",
		Authentication: profile.AuthenticationPassword,
		Credentials:    port.SSHCredentials{Password: []byte("secret")},
		Columns:        100, Rows: 30,
	}

	first, err := factory.OpenSSH(ctx, spec)
	if err != nil {
		t.Fatalf("open first terminal: %v", err)
	}
	second, err := factory.OpenSSH(ctx, spec)
	if err != nil {
		_ = first.Close()
		t.Fatalf("open second terminal: %v", err)
	}
	if connections := server.connections.Load(); connections != 1 {
		t.Fatalf("expected one authenticated connection, got %d", connections)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first terminal: %v", err)
	}
	if _, err := second.Write([]byte("still-open\n")); err != nil {
		t.Fatalf("write second terminal after closing first: %v", err)
	}
	output := make([]byte, len("still-open\n"))
	if _, err := io.ReadFull(second, output); err != nil {
		t.Fatalf("read second terminal after closing first: %v", err)
	}
	if string(output) != "still-open\n" {
		t.Fatalf("unexpected second terminal output %q", output)
	}
	if _, err := second.Write([]byte("exit\n")); err != nil {
		t.Fatalf("request second terminal exit: %v", err)
	}
	status, err := second.Wait()
	if err != nil {
		t.Fatalf("wait for second terminal: %v", err)
	}
	if status.Code != 7 {
		t.Fatalf("unexpected second terminal status: %#v", status)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second terminal: %v", err)
	}
}

type fixedHostKey struct {
	key ssh.PublicKey
}

func (f fixedHostKey) HostKeyCallback(string, int) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if !bytes.Equal(f.key.Marshal(), key.Marshal()) {
			return errors.New("unexpected host key")
		}
		return nil
	}
}

type testSSHServer struct {
	listener    net.Listener
	hostKey     ssh.PublicKey
	config      *ssh.ServerConfig
	resize      chan [2]uint32
	done        chan struct{}
	once        sync.Once
	connections atomic.Int32
}

func newTestSSHServer(t *testing.T) *testSSHServer {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("create host signer: %v", err)
	}
	hostKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatalf("create public host key: %v", err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if metadata.User() == "tester" && string(password) == "secret" {
				return nil, nil
			}
			return nil, errors.New("authentication rejected")
		},
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit loopback listeners")
			return nil
		}
		t.Fatalf("listen: %v", err)
	}
	server := &testSSHServer{
		listener: listener, hostKey: hostKey, config: config,
		resize: make(chan [2]uint32, 4), done: make(chan struct{}),
	}
	go server.serve()
	return server
}

func (s *testSSHServer) Address() string {
	return s.listener.Addr().String()
}

func (s *testSSHServer) Close() {
	s.once.Do(func() { _ = s.listener.Close() })
	select {
	case <-s.done:
	case <-time.After(time.Second):
	}
}

func (s *testSSHServer) serve() {
	defer close(s.done)
	raw, err := s.listener.Accept()
	if err != nil {
		return
	}
	s.connections.Add(1)
	defer raw.Close()
	connection, channels, requests, err := ssh.NewServerConn(raw, s.config)
	if err != nil {
		return
	}
	defer connection.Close()
	go ssh.DiscardRequests(requests)
	var sessions sync.WaitGroup
	for incoming := range channels {
		if incoming.ChannelType() != "session" {
			_ = incoming.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, channelRequests, err := incoming.Accept()
		if err != nil {
			continue
		}
		sessions.Add(1)
		go func() {
			defer sessions.Done()
			s.handleSession(channel, channelRequests)
		}()
	}
	sessions.Wait()
}

func (s *testSSHServer) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	started := make(chan struct{})
	var startedOnce sync.Once
	go func() {
		for request := range requests {
			switch request.Type {
			case "pty-req", "shell":
				_ = request.Reply(true, nil)
				if request.Type == "shell" {
					startedOnce.Do(func() { close(started) })
				}
			case "window-change":
				var size struct {
					Columns, Rows, WidthPixels, HeightPixels uint32
				}
				if err := ssh.Unmarshal(request.Payload, &size); err == nil {
					s.resize <- [2]uint32{size.Columns, size.Rows}
				}
			default:
				_ = request.Reply(false, nil)
			}
		}
	}()
	<-started

	buffer := make([]byte, 1024)
	for {
		n, err := channel.Read(buffer)
		if n > 0 {
			if string(buffer[:n]) == "exit\n" {
				_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 7}))
				return
			}
			if _, writeErr := channel.Write(buffer[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *testSSHServer) String() string {
	return fmt.Sprintf("test SSH server at %s", s.Address())
}
