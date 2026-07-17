package sshterminal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
