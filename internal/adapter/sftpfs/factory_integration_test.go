package sftpfs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"shh-h/internal/adapter/sshclient"
	"shh-h/internal/adapter/sshterminal"
	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

func TestFactoryRemoteFilesystemOperations(t *testing.T) {
	server := startSFTPServer(t)
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.listener.Addr().String())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	sshPort, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	dialer := sshterminal.NewDialer(fixedVerifier{key: server.hostKey})
	clients := sshclient.NewPool(dialer, nil)
	defer clients.Shutdown()
	filesystem, err := NewFactory(clients).OpenRemoteFilesystem(context.Background(), port.SSHTerminalSpec{
		Host: host, Port: sshPort, Username: "tester",
		Authentication: profile.AuthenticationPassword,
		Credentials:    port.SSHCredentials{Password: []byte("secret")},
	})
	if err != nil {
		t.Fatalf("open remote filesystem: %v", err)
	}
	defer filesystem.Close()

	entries, err := filesystem.ReadDirectory(context.Background(), ".")
	if err != nil {
		t.Fatalf("list remote directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "fixture.txt" {
		t.Fatalf("unexpected remote entries: %#v", entries)
	}
	source, size, err := filesystem.OpenRead(context.Background(), entries[0].Path, 0)
	if err != nil {
		t.Fatalf("open remote fixture: %v", err)
	}
	data, err := io.ReadAll(source)
	_ = source.Close()
	if err != nil || string(data) != "fixture" || size != int64(len("fixture")) {
		t.Fatalf("unexpected remote fixture: data=%q size=%d err=%v", data, size, err)
	}
	resumedSource, resumedSize, err := filesystem.OpenRead(context.Background(), entries[0].Path, 3)
	if err != nil {
		t.Fatalf("open remote fixture at offset: %v", err)
	}
	resumedData, err := io.ReadAll(resumedSource)
	_ = resumedSource.Close()
	if err != nil || string(resumedData) != "ture" || resumedSize != int64(len("fixture")) {
		t.Fatalf("unexpected resumed read: data=%q size=%d err=%v", resumedData, resumedSize, err)
	}

	uploadPath := filepath.ToSlash(filepath.Join(filesystem.WorkingDirectory(), "upload.txt"))
	destination, err := filesystem.OpenWrite(context.Background(), uploadPath, 0)
	if err != nil {
		t.Fatalf("open remote upload: %v", err)
	}
	if _, err := destination.Write([]byte("upload")); err != nil {
		t.Fatalf("write remote upload: %v", err)
	}
	if err := destination.Close(); err != nil {
		t.Fatalf("close remote upload: %v", err)
	}
	destination, err = filesystem.OpenWrite(context.Background(), uploadPath, int64(len("upload")))
	if err != nil {
		t.Fatalf("resume remote upload: %v", err)
	}
	if _, err := destination.Write([]byte("ed")); err != nil {
		t.Fatalf("write resumed remote upload: %v", err)
	}
	if err := destination.Close(); err != nil {
		t.Fatalf("close resumed remote upload: %v", err)
	}
	uploaded, uploadedSize, err := filesystem.OpenRead(context.Background(), uploadPath, 0)
	if err != nil {
		t.Fatalf("open resumed upload: %v", err)
	}
	uploadedData, err := io.ReadAll(uploaded)
	_ = uploaded.Close()
	if err != nil || string(uploadedData) != "uploaded" || uploadedSize != int64(len("uploaded")) {
		t.Fatalf("unexpected resumed upload: data=%q size=%d err=%v", uploadedData, uploadedSize, err)
	}
	renamedPath := filepath.ToSlash(filepath.Join(filesystem.WorkingDirectory(), "renamed.txt"))
	if err := filesystem.Rename(context.Background(), uploadPath, renamedPath); err != nil {
		t.Fatalf("rename remote upload: %v", err)
	}
	if err := filesystem.Remove(context.Background(), renamedPath); err != nil {
		t.Fatalf("remove remote upload: %v", err)
	}
}

type fixedVerifier struct{ key ssh.PublicKey }

func (v fixedVerifier) HostKeyCallback(string, int) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if !bytes.Equal(v.key.Marshal(), key.Marshal()) {
			return errors.New("unexpected host key")
		}
		return nil
	}
}

type sftpTestServer struct {
	listener net.Listener
	hostKey  ssh.PublicKey
	config   *ssh.ServerConfig
	root     string
	done     chan struct{}
	once     sync.Once
}

func startSFTPServer(t *testing.T) *sftpTestServer {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
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
		t.Fatalf("create host public key: %v", err)
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
			return nil
		}
		t.Fatalf("listen: %v", err)
	}
	server := &sftpTestServer{listener: listener, hostKey: hostKey, config: config, root: root, done: make(chan struct{})}
	go server.serve()
	return server
}

func (s *sftpTestServer) Close() {
	s.once.Do(func() { _ = s.listener.Close() })
	select {
	case <-s.done:
	case <-time.After(time.Second):
	}
}

func (s *sftpTestServer) serve() {
	defer close(s.done)
	raw, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer raw.Close()
	connection, channels, requests, err := ssh.NewServerConn(raw, s.config)
	if err != nil {
		return
	}
	defer connection.Close()
	go ssh.DiscardRequests(requests)
	for incoming := range channels {
		if incoming.ChannelType() != "session" {
			_ = incoming.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, channelRequests, err := incoming.Accept()
		if err != nil {
			continue
		}
		for request := range channelRequests {
			if request.Type != "subsystem" {
				_ = request.Reply(false, nil)
				continue
			}
			var payload struct{ Name string }
			if ssh.Unmarshal(request.Payload, &payload) != nil || payload.Name != "sftp" {
				_ = request.Reply(false, nil)
				continue
			}
			_ = request.Reply(true, nil)
			server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(s.root))
			if err == nil {
				_ = server.Serve()
				_ = server.Close()
			}
			_ = channel.Close()
			break
		}
	}
}
