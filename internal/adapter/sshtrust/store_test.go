package sshtrust

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"shh-h/internal/domain/profile"
	"shh-h/internal/domain/sshconnection"
)

func TestTrustPermanentHostKeyAndRejectChangedKey(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "known_hosts")
	store := NewAt(path, filepath.Join(directory, "missing-user-known-hosts"))
	key := testPublicKey(t)
	address := "example.com:22"

	status, err := store.status(address, key)
	if err != nil || status != sshconnection.HostKeyUnknown {
		t.Fatalf("expected unknown key, got status=%q err=%v", status, err)
	}
	store.pending["challenge"] = pendingKey{
		leaseID: "lease", address: address, key: key, expiresAt: time.Now().Add(time.Minute),
	}
	if err := store.Trust("lease", "challenge", true); err != nil {
		t.Fatalf("trust host key: %v", err)
	}
	status, err = store.status(address, key)
	if err != nil || status != sshconnection.HostKeyKnown {
		t.Fatalf("expected known key, got status=%q err=%v", status, err)
	}
	status, err = store.status(address, testPublicKey(t))
	if err != nil || status != sshconnection.HostKeyChanged {
		t.Fatalf("expected changed key, got status=%q err=%v", status, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat application known hosts: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected private known-host permissions, got %o", info.Mode().Perm())
	}
}

func TestTrustChallengeIsBoundToFrontendLease(t *testing.T) {
	store := NewAt(filepath.Join(t.TempDir(), "known_hosts"), "")
	store.pending["challenge"] = pendingKey{
		leaseID: "lease-one", address: "example.com:22", key: testPublicKey(t), expiresAt: time.Now().Add(time.Minute),
	}
	if err := store.Trust("lease-two", "challenge", false); err == nil {
		t.Fatal("expected a challenge from another lease to be rejected")
	}
}

func TestProbeCapturesServerKeyWithoutAuthentication(t *testing.T) {
	listener, address, cleanup := startProbeServer(t)
	defer cleanup()
	defer listener.Close()

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split test address: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse test port: %v", err)
	}
	store := NewAt(filepath.Join(t.TempDir(), "known_hosts"), "")
	info, err := store.Probe(context.Background(), "lease", profile.Profile{
		ID: "test", Name: "Test", Protocol: profile.ProtocolSSH, Host: host, Port: port,
	})
	if err != nil {
		t.Fatalf("probe host key: %v", err)
	}
	if info.Status != sshconnection.HostKeyUnknown || info.ChallengeID == "" || info.Fingerprint == "" {
		t.Fatalf("unexpected probe result: %#v", info)
	}
}

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatalf("create ssh public key: %v", err)
	}
	return key
}

func startProbeServer(t *testing.T) (net.Listener, string, func()) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("create server signer: %v", err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit loopback listeners")
			return nil, "", func() {}
		}
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		server, channels, requests, err := ssh.NewServerConn(connection, config)
		if err == nil {
			server.Close()
		}
		if channels != nil {
			for channel := range channels {
				channel.Reject(ssh.Prohibited, "probe only")
			}
		}
		if requests != nil {
			for request := range requests {
				request.Reply(false, nil)
			}
		}
	}()
	cleanup := func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
	return listener, listener.Addr().String(), cleanup
}
