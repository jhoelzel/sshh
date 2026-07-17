package tunnel_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"shh-h/internal/adapter/sshclient"
	"shh-h/internal/adapter/sshterminal"
	"shh-h/internal/domain/profile"
	tunneldomain "shh-h/internal/domain/tunnel"
	"shh-h/internal/port"
	tunnelusecase "shh-h/internal/usecase/tunnel"
)

func TestRealSSHTunnelForwarding(t *testing.T) {
	server := startForwardingSSHServer(t)
	defer server.Close()
	target := startIntegrationEchoServer(t)
	defer target.Close()
	host, portText, err := net.SplitHostPort(server.Address())
	if err != nil {
		t.Fatalf("split SSH address: %v", err)
	}
	sshPort, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse SSH port: %v", err)
	}
	_, targetPortText, err := net.SplitHostPort(target.Addr().String())
	if err != nil {
		t.Fatalf("split target address: %v", err)
	}
	targetPort, err := strconv.Atoi(targetPortText)
	if err != nil {
		t.Fatalf("parse target port: %v", err)
	}
	selected := profile.Profile{
		ID: "ssh", Name: "SSH", Protocol: profile.ProtocolSSH,
		Host: host, Port: sshPort, Username: "tester", Authentication: profile.AuthenticationPassword,
	}
	dialer := sshterminal.NewDialer(integrationHostKey{key: server.hostKey})
	clients := sshclient.NewPool(dialer, nil)
	defer clients.Shutdown()

	tests := []struct {
		name   string
		config tunneldomain.Config
		socks  bool
	}{
		{name: "local", config: integrationConfig("local", tunneldomain.KindLocal, targetPort)},
		{name: "remote", config: integrationConfig("remote", tunneldomain.KindRemote, targetPort)},
		{name: "dynamic", config: integrationConfig("dynamic", tunneldomain.KindDynamic, 0), socks: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &integrationRepository{configs: []tunneldomain.Config{test.config}}
			service, err := tunnelusecase.NewService(repo, integrationProfiles{selected: selected}, clients)
			if err != nil {
				t.Fatalf("create tunnel service: %v", err)
			}
			snapshot, err := service.Start(context.Background(), "lease", test.config.ID, port.SSHCredentials{Password: []byte("secret")})
			if err != nil {
				t.Fatalf("start %s tunnel: %v", test.name, err)
			}
			defer service.Stop("lease", test.config.ID)
			if test.socks {
				assertSOCKSRoundTrip(t, snapshot.BoundAddress, targetPort)
			} else {
				assertIntegrationRoundTrip(t, snapshot.BoundAddress)
			}
		})
	}
}

type integrationRepository struct{ configs []tunneldomain.Config }

func (r *integrationRepository) LoadTunnels() ([]tunneldomain.Config, error) {
	return append([]tunneldomain.Config{}, r.configs...), nil
}
func (r *integrationRepository) SaveTunnels(configs []tunneldomain.Config) error {
	r.configs = append([]tunneldomain.Config{}, configs...)
	return nil
}

type integrationProfiles struct{ selected profile.Profile }

func (p integrationProfiles) Find(id string) (profile.Profile, bool) {
	return p.selected, id == p.selected.ID
}

type integrationHostKey struct{ key ssh.PublicKey }

func (v integrationHostKey) HostKeyCallback(string, int) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if !bytes.Equal(v.key.Marshal(), key.Marshal()) {
			return errors.New("unexpected host key")
		}
		return nil
	}
}

func integrationConfig(id string, kind tunneldomain.Kind, destinationPort int) tunneldomain.Config {
	config := tunneldomain.Config{
		ID: id, Name: id, ProfileID: "ssh", Kind: kind, BindAddress: "127.0.0.1",
		DestinationHost: "127.0.0.1", DestinationPort: destinationPort,
	}
	if kind == tunneldomain.KindDynamic {
		config.DestinationHost = ""
	}
	return config.WithDefaults(time.Now().UTC())
}

type forwardingSSHServer struct {
	listener net.Listener
	hostKey  ssh.PublicKey
	config   *ssh.ServerConfig
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	clients  map[*ssh.ServerConn]struct{}
}

func startForwardingSSHServer(t *testing.T) *forwardingSSHServer {
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
	config := &ssh.ServerConfig{PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if metadata.User() == "tester" && string(password) == "secret" {
			return nil, nil
		}
		return nil, errors.New("authentication rejected")
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit loopback listeners")
		}
		t.Fatalf("listen for SSH: %v", err)
	}
	server := &forwardingSSHServer{
		listener: listener, hostKey: hostKey, config: config,
		done: make(chan struct{}), clients: make(map[*ssh.ServerConn]struct{}),
	}
	go server.serve()
	return server
}

func (s *forwardingSSHServer) Address() string { return s.listener.Addr().String() }

func (s *forwardingSSHServer) Close() {
	s.once.Do(func() {
		_ = s.listener.Close()
		s.mu.Lock()
		for client := range s.clients {
			_ = client.Close()
		}
		s.mu.Unlock()
	})
	select {
	case <-s.done:
	case <-time.After(time.Second):
	}
}

func (s *forwardingSSHServer) serve() {
	defer close(s.done)
	var clients sync.WaitGroup
	defer clients.Wait()
	for {
		raw, err := s.listener.Accept()
		if err != nil {
			return
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			s.serveClient(raw)
		}()
	}
}

func (s *forwardingSSHServer) serveClient(raw net.Conn) {
	defer raw.Close()
	connection, channels, requests, err := ssh.NewServerConn(raw, s.config)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.clients[connection] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, connection)
		s.mu.Unlock()
		_ = connection.Close()
	}()
	forwarded := make(map[string]net.Listener)
	var forwardedMu sync.Mutex
	go func() {
		for request := range requests {
			switch request.Type {
			case "tcpip-forward":
				var payload forwardRequest
				if ssh.Unmarshal(request.Payload, &payload) != nil {
					_ = request.Reply(false, nil)
					continue
				}
				listener, err := net.Listen("tcp", net.JoinHostPort(payload.Address, strconv.Itoa(int(payload.Port))))
				if err != nil {
					_ = request.Reply(false, nil)
					continue
				}
				actualPort := uint32(listener.Addr().(*net.TCPAddr).Port)
				key := net.JoinHostPort(payload.Address, strconv.Itoa(int(actualPort)))
				forwardedMu.Lock()
				forwarded[key] = listener
				forwardedMu.Unlock()
				var reply []byte
				if payload.Port == 0 {
					reply = ssh.Marshal(struct{ Port uint32 }{Port: actualPort})
				}
				_ = request.Reply(true, reply)
				go serveForwardedListener(connection, listener, payload.Address, actualPort)
			case "cancel-tcpip-forward":
				var payload forwardRequest
				_ = ssh.Unmarshal(request.Payload, &payload)
				key := net.JoinHostPort(payload.Address, strconv.Itoa(int(payload.Port)))
				forwardedMu.Lock()
				listener := forwarded[key]
				delete(forwarded, key)
				forwardedMu.Unlock()
				if listener != nil {
					_ = listener.Close()
				}
				_ = request.Reply(listener != nil, nil)
			default:
				_ = request.Reply(false, nil)
			}
		}
	}()
	for incoming := range channels {
		if incoming.ChannelType() != "direct-tcpip" {
			_ = incoming.Reject(ssh.UnknownChannelType, "forwarding only")
			continue
		}
		go handleDirectChannel(incoming)
	}
	forwardedMu.Lock()
	for _, listener := range forwarded {
		_ = listener.Close()
	}
	forwardedMu.Unlock()
}

type forwardRequest struct {
	Address string
	Port    uint32
}

type directRequest struct {
	DestinationAddress string
	DestinationPort    uint32
	OriginAddress      string
	OriginPort         uint32
}

func handleDirectChannel(incoming ssh.NewChannel) {
	var request directRequest
	if ssh.Unmarshal(incoming.ExtraData(), &request) != nil {
		_ = incoming.Reject(ssh.ConnectionFailed, "invalid forwarding request")
		return
	}
	destination, err := net.Dial("tcp", net.JoinHostPort(request.DestinationAddress, strconv.Itoa(int(request.DestinationPort))))
	if err != nil {
		_ = incoming.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	channel, requests, err := incoming.Accept()
	if err != nil {
		_ = destination.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	go relayIntegration(channel, destination)
}

func serveForwardedListener(connection *ssh.ServerConn, listener net.Listener, address string, port uint32) {
	for {
		incoming, err := listener.Accept()
		if err != nil {
			return
		}
		originHost, originPortText, _ := net.SplitHostPort(incoming.RemoteAddr().String())
		originPort, _ := strconv.Atoi(originPortText)
		payload := ssh.Marshal(struct {
			ConnectedAddress string
			ConnectedPort    uint32
			OriginAddress    string
			OriginPort       uint32
		}{address, port, originHost, uint32(originPort)})
		channel, requests, err := connection.OpenChannel("forwarded-tcpip", payload)
		if err != nil {
			_ = incoming.Close()
			continue
		}
		go ssh.DiscardRequests(requests)
		go relayIntegration(channel, incoming)
	}
}

func relayIntegration(left io.ReadWriteCloser, right io.ReadWriteCloser) {
	defer left.Close()
	defer right.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(left, right); done <- struct{}{} }()
	go func() { _, _ = io.Copy(right, left); done <- struct{}{} }()
	<-done
}

type integrationEchoServer struct {
	listener net.Listener
	done     chan struct{}
}

func startIntegrationEchoServer(t *testing.T) *integrationEchoServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for echo: %v", err)
	}
	server := &integrationEchoServer{listener: listener, done: make(chan struct{})}
	go func() {
		defer close(server.done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer connection.Close(); _, _ = io.Copy(connection, connection) }()
		}
	}()
	return server
}

func (s *integrationEchoServer) Addr() net.Addr { return s.listener.Addr() }
func (s *integrationEchoServer) Close() {
	_ = s.listener.Close()
	<-s.done
}

func assertIntegrationRoundTrip(t *testing.T, address string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("dial forwarded address: %v", err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("real-ssh")); err != nil {
		t.Fatalf("write forwarded payload: %v", err)
	}
	payload := make([]byte, len("real-ssh"))
	if _, err := io.ReadFull(connection, payload); err != nil || string(payload) != "real-ssh" {
		t.Fatalf("forwarded round trip: payload=%q err=%v", payload, err)
	}
}

func assertSOCKSRoundTrip(t *testing.T, address string, targetPort int) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("dial SOCKS tunnel: %v", err)
	}
	defer connection.Close()
	_, _ = connection.Write([]byte{0x05, 0x01, 0x00})
	response := make([]byte, 2)
	if _, err := io.ReadFull(connection, response); err != nil || response[1] != 0 {
		t.Fatalf("SOCKS greeting: response=%v err=%v", response, err)
	}
	request := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, 0, 0}
	binary.BigEndian.PutUint16(request[8:], uint16(targetPort))
	_, _ = connection.Write(request)
	reply := make([]byte, 10)
	if _, err := io.ReadFull(connection, reply); err != nil || reply[1] != 0 {
		t.Fatalf("SOCKS connect: response=%v err=%v", reply, err)
	}
	if _, err := connection.Write([]byte("real-socks")); err != nil {
		t.Fatalf("write SOCKS payload: %v", err)
	}
	payload := make([]byte, len("real-socks"))
	if _, err := io.ReadFull(connection, payload); err != nil || string(payload) != "real-socks" {
		t.Fatalf("SOCKS round trip: payload=%q err=%v", payload, err)
	}
}
