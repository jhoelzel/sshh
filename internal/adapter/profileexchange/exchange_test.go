package profileexchange

import (
	"bytes"
	"strings"
	"testing"
	"time"

	profiledomain "shh-h/internal/domain/profile"
)

func TestPortableJSONRoundTripExcludesRuntimeIdentity(t *testing.T) {
	profiles := []profiledomain.Profile{{
		ID: "private-runtime-id", Name: "Production", Protocol: profiledomain.ProtocolSSH,
		Host: "prod.example.com", Port: 2222, Username: "deploy",
		Authentication: profiledomain.AuthenticationKey, IdentityFile: "~/.ssh/id_ed25519",
		Environment: map[string]string{"LANG": "en_US.UTF-8"}, Tags: []string{"prod"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}}

	data, err := Encode(profiles)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if bytes.Contains(data, []byte("private-runtime-id")) || bytes.Contains(data, []byte("created_at")) {
		t.Fatalf("portable data contains runtime identity: %s", data)
	}

	parsed, err := Parse("profiles.json", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Format != FormatJSON || len(parsed.Profiles) != 1 {
		t.Fatalf("unexpected parse result: %#v", parsed)
	}
	profile := parsed.Profiles[0]
	if profile.ID != "" || profile.Host != "prod.example.com" || profile.Environment["LANG"] != "en_US.UTF-8" {
		t.Fatalf("unexpected round trip profile: %#v", profile)
	}
}

func TestPortableJSONIsStrict(t *testing.T) {
	for _, data := range []string{
		`{"version":1,"profiles":[],"unexpected":true}`,
		`{"version":2,"profiles":[]}`,
		`{"version":1,"profiles":[]} {}`,
	} {
		if _, err := Parse("profiles.json", []byte(data)); err == nil {
			t.Fatalf("expected strict parse failure for %s", data)
		}
	}
}

func TestOpenSSHImportUsesFirstApplicableValue(t *testing.T) {
	data := []byte(`
Host production
  HostName prod.example.com
  User deploy
  IdentityFile "~/.ssh/production key"
  IdentityFile ~/.ssh/fallback

Host staging
  HostName=staging.example.com

Host *
  User fallback
  Port 2222
`)
	result, err := Parse("config", data)
	if err != nil {
		t.Fatalf("parse OpenSSH config: %v", err)
	}
	if result.Format != FormatOpenSSH || len(result.Profiles) != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	production := result.Profiles[0]
	if production.Name != "production" || production.Host != "prod.example.com" || production.Username != "deploy" || production.Port != 2222 || production.IdentityFile != "~/.ssh/production key" {
		t.Fatalf("unexpected production profile: %#v", production)
	}
	staging := result.Profiles[1]
	if staging.Username != "fallback" || staging.Port != 2222 {
		t.Fatalf("global defaults were not applied: %#v", staging)
	}
	if !warningsContain(result.Warnings, "only the first was imported") {
		t.Fatalf("missing multiple identity warning: %#v", result.Warnings)
	}
}

func TestOpenSSHImportReportsPatternsMatchAndUnsupportedDirectives(t *testing.T) {
	data := []byte(`
Host prod-*
  User deploy
Host direct
  HostName direct.example.com
  ForwardAgent yes
Match user deploy
  Port 2200
Host safe
  HostName safe.example.com
`)
	result, err := Parse("ssh_config", data)
	if err != nil {
		t.Fatalf("parse OpenSSH config: %v", err)
	}
	if len(result.Profiles) != 2 || result.Profiles[0].Name != "direct" || result.Profiles[1].Name != "safe" {
		t.Fatalf("unexpected profiles: %#v", result.Profiles)
	}
	for _, fragment := range []string{"wildcard Host pattern", "unsupported directive", "Match blocks", "inside a Match block"} {
		if !warningsContain(result.Warnings, fragment) {
			t.Fatalf("missing %q warning in %#v", fragment, result.Warnings)
		}
	}
}

func TestOpenSSHImportSkipsUnsafeOrInvalidConnections(t *testing.T) {
	data := []byte(`
Host proxied
  HostName internal.example.com
  ProxyJump bastion
Host invalid-port
  Port 70000
Host proxy-disabled
  ProxyJump none
Host tokenized-key
  IdentityFile %d/.ssh/id_ed25519
Host valid
  HostName valid.example.com
`)
	result, err := Parse("config", data)
	if err != nil {
		t.Fatalf("parse OpenSSH config: %v", err)
	}
	if len(result.Profiles) != 3 || result.Profiles[0].Name != "proxy-disabled" || result.Profiles[1].Name != "tokenized-key" || result.Profiles[1].IdentityFile != "" || result.Profiles[2].Name != "valid" {
		t.Fatalf("unsafe profiles were imported: %#v", result.Profiles)
	}
	if !warningsContain(result.Warnings, "cannot be represented safely") || !warningsContain(result.Warnings, "invalid Port") || !warningsContain(result.Warnings, "path expansion") {
		t.Fatalf("missing skip diagnostics: %#v", result.Warnings)
	}
}

func TestOpenSSHImportHonorsWildcardDefaultsAndNegation(t *testing.T) {
	data := []byte(`
Host app !app-admin
  Port 2200
Host app app-admin
  HostName app.example.com
Host *
  User operator
`)
	result, err := Parse("config", data)
	if err != nil {
		t.Fatalf("parse OpenSSH config: %v", err)
	}
	if len(result.Profiles) != 2 {
		t.Fatalf("expected two aliases: %#v", result.Profiles)
	}
	if result.Profiles[0].Port != 2200 || result.Profiles[1].Port != 22 || result.Profiles[1].Username != "operator" {
		t.Fatalf("patterns were evaluated incorrectly: %#v", result.Profiles)
	}
}

func TestOpenSSHImportReportsUnterminatedQuotes(t *testing.T) {
	result, err := Parse("config", []byte("Host broken\n  IdentityFile \"unterminated\nHost valid\n"))
	if err != nil {
		t.Fatalf("parse OpenSSH config: %v", err)
	}
	if len(result.Profiles) != 2 || !warningsContain(result.Warnings, "unterminated quoted value") {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestOpenSSHImportReportsMissingConcreteHosts(t *testing.T) {
	result, err := Parse("config", []byte("Host *\n  User operator\n"))
	if err != nil {
		t.Fatalf("parse OpenSSH config: %v", err)
	}
	if len(result.Profiles) != 0 || !warningsContain(result.Warnings, "no concrete Host entries") {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestWildcardMatchHandlesUnicodeAliases(t *testing.T) {
	if !wildcardMatch("m?nchen-*", "münchen-prod") {
		t.Fatal("unicode host alias did not match")
	}
}

func warningsContain(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}
	return false
}
