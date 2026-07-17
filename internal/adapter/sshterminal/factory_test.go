package sshterminal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"shh-h/internal/domain/profile"
	"shh-h/internal/domain/sshconnection"
	"shh-h/internal/port"
)

func TestInspectAuthenticationDetectsEncryptedAndPlainKeys(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	directory := t.TempDir()
	plainPath := filepath.Join(directory, "plain")
	plainBlock, err := ssh.MarshalPrivateKey(private, "test")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	if err := os.WriteFile(plainPath, pem.EncodeToMemory(plainBlock), 0o600); err != nil {
		t.Fatalf("write plain key: %v", err)
	}
	encryptedPath := filepath.Join(directory, "encrypted")
	encryptedBlock, err := ssh.MarshalPrivateKeyWithPassphrase(private, "test", []byte("secret"))
	if err != nil {
		t.Fatalf("marshal encrypted key: %v", err)
	}
	if err := os.WriteFile(encryptedPath, pem.EncodeToMemory(encryptedBlock), 0o600); err != nil {
		t.Fatalf("write encrypted key: %v", err)
	}

	plain, err := InspectAuthentication(profile.Profile{Authentication: profile.AuthenticationKey, IdentityFile: plainPath})
	if err != nil || plain.Secret != sshconnection.SecretNone {
		t.Fatalf("inspect plain key: info=%#v err=%v", plain, err)
	}
	encrypted, err := InspectAuthentication(profile.Profile{Authentication: profile.AuthenticationKey, IdentityFile: encryptedPath})
	if err != nil || encrypted.Secret != sshconnection.SecretPassphrase {
		t.Fatalf("inspect encrypted key: info=%#v err=%v", encrypted, err)
	}
	if _, err := parsePrivateKey(encryptedPath, nil); !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("expected passphrase requirement, got %v", err)
	}
	if _, err := parsePrivateKey(encryptedPath, []byte("secret")); err != nil {
		t.Fatalf("decrypt private key: %v", err)
	}
}

func TestPasswordAuthenticationInspectionRequiresPrompt(t *testing.T) {
	info, err := InspectAuthentication(profile.Profile{Authentication: profile.AuthenticationPassword})
	if err != nil {
		t.Fatalf("inspect password authentication: %v", err)
	}
	if info.Secret != sshconnection.SecretPassword {
		t.Fatalf("unexpected requirement: %#v", info)
	}
}

func TestBuildAuthMethodsRequiresCredentials(t *testing.T) {
	_, _, err := buildAuthMethods(context.Background(), sshSpec(profile.AuthenticationPassword))
	if !errors.Is(err, ErrCredentialsRequired) {
		t.Fatalf("expected credentials requirement, got %v", err)
	}
}

func sshSpec(authentication profile.Authentication) port.SSHTerminalSpec {
	return port.SSHTerminalSpec{Authentication: authentication}
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

type maintenanceClient struct {
	mu        sync.Mutex
	requests  int
	respond   func() error
	closed    chan struct{}
	requested chan struct{}
	closeOnce sync.Once
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
