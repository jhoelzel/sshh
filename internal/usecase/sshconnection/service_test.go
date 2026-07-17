package sshconnection

import (
	"context"
	"strings"
	"testing"

	profiledomain "shh-h/internal/domain/profile"
	sshconnectiondomain "shh-h/internal/domain/sshconnection"
)

type recordingTrustStore struct {
	selected profiledomain.Profile
}

func (store *recordingTrustStore) Probe(_ context.Context, _ string, selected profiledomain.Profile) (sshconnectiondomain.HostKeyInfo, error) {
	store.selected = selected
	return sshconnectiondomain.HostKeyInfo{Status: sshconnectiondomain.HostKeyKnown, Host: selected.Host}, nil
}

func (*recordingTrustStore) Trust(string, string, bool) error { return nil }

type recordingInspector struct {
	selected profiledomain.Profile
}

func (inspector *recordingInspector) InspectAuthentication(selected profiledomain.Profile) (sshconnectiondomain.AuthenticationInfo, error) {
	inspector.selected = selected
	return sshconnectiondomain.AuthenticationInfo{Secret: sshconnectiondomain.SecretPassword}, nil
}

func TestQuickProfileIsTransientNormalizedAndDelegated(t *testing.T) {
	trust := &recordingTrustStore{}
	inspector := &recordingInspector{}
	service := NewService(nil, nil, nil, trust, inspector)
	candidate := profiledomain.Profile{
		ID: "persisted-looking-id", Name: "Pretend saved profile", Protocol: profiledomain.ProtocolLocal,
		Host: " [2001:db8::1] ", Port: 2222, Username: " deploy ",
		Authentication: profiledomain.AuthenticationPassword, Tags: []string{"must-not-survive"},
	}

	selected, info, err := service.ProbeQuickHostKey(context.Background(), "lease", candidate)
	if err != nil {
		t.Fatalf("probe quick host: %v", err)
	}
	if info.Status != sshconnectiondomain.HostKeyKnown || selected.Host != "2001:db8::1" || selected.Name != "deploy@[2001:db8::1]:2222" {
		t.Fatalf("unexpected normalized profile: %#v info=%#v", selected, info)
	}
	if !strings.HasPrefix(selected.ID, "quick-ssh-") || selected.ID == candidate.ID || len(selected.Tags) != 0 {
		t.Fatalf("quick profile retained persisted identity: %#v", selected)
	}
	if trust.selected.ID != selected.ID {
		t.Fatalf("trust store received a different profile: %#v", trust.selected)
	}

	authentication, err := service.InspectQuickAuthentication(candidate)
	if err != nil {
		t.Fatalf("inspect quick authentication: %v", err)
	}
	if authentication.Secret != sshconnectiondomain.SecretPassword || inspector.selected.ID != selected.ID {
		t.Fatalf("unexpected authentication delegation: %#v profile=%#v", authentication, inspector.selected)
	}
}

func TestQuickProfileRejectsAmbiguousOrInvalidTargets(t *testing.T) {
	tests := []profiledomain.Profile{
		{Host: "https://example.com", Port: 22},
		{Host: "example.com:2222", Port: 22},
		{Host: "example.com\nother", Port: 22},
		{Host: "example.com", Port: 22, Username: "bad user"},
		{Host: "example.com", Port: 70000},
		{Host: "example.com", Port: 22, Authentication: profiledomain.AuthenticationKey},
	}
	for _, candidate := range tests {
		if _, err := normalizeQuickProfile(candidate); err == nil {
			t.Fatalf("expected invalid quick profile to fail: %#v", candidate)
		}
	}
}

func TestQuickProfileDefaultsToPort22AndAutoAuthentication(t *testing.T) {
	selected, err := normalizeQuickProfile(profiledomain.Profile{Host: "example.com"})
	if err != nil {
		t.Fatalf("normalize quick profile: %v", err)
	}
	if selected.Port != 22 || selected.Authentication != profiledomain.AuthenticationAuto || selected.Endpoint() != "example.com:22" {
		t.Fatalf("unexpected defaults: %#v", selected)
	}
}
