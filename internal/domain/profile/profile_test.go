package profile

import (
	"strings"
	"testing"
)

func TestEndpointFormatsIPv6WithoutAmbiguity(t *testing.T) {
	profile := Profile{Protocol: ProtocolSSH, Host: "[2001:db8::1]", Port: 2222, Username: "deploy"}
	if got := profile.Endpoint(); got != "deploy@[2001:db8::1]:2222" {
		t.Fatalf("unexpected endpoint: %q", got)
	}
}

func TestValidateAcceptsPortableEnvironmentOverrides(t *testing.T) {
	item := Profile{
		ID: "local", Name: "Local", Protocol: ProtocolLocal,
		Environment: map[string]string{"LANG": "en_US.UTF-8", "PATH": "", "_SHHH_TEST_2": "line one\nline two"},
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("validate portable environment: %v", err)
	}
}

func TestValidateRejectsUnsafeEnvironmentOverrides(t *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		message     string
	}{
		{name: "empty name", environment: map[string]string{"": "value"}, message: "must start with a letter or underscore"},
		{name: "leading digit", environment: map[string]string{"2FA_TOKEN": "value"}, message: "must start with a letter or underscore"},
		{name: "separator", environment: map[string]string{"BAD=NAME": "value"}, message: "must start with a letter or underscore"},
		{name: "case collision", environment: map[string]string{"Path": "one", "PATH": "two"}, message: "differ only by case"},
		{name: "managed terminal type", environment: map[string]string{"term": "vt100"}, message: "managed by shh-h"},
		{name: "managed color terminal", environment: map[string]string{"COLORTERM": "false"}, message: "managed by shh-h"},
		{name: "managed session id", environment: map[string]string{"SHHH_SESSION_ID": "fixed"}, message: "managed by shh-h"},
		{name: "null value", environment: map[string]string{"VALID": "before\x00after"}, message: "contains a null byte"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := Profile{ID: "local", Name: "Local", Protocol: ProtocolLocal, Environment: test.environment}
			err := item.Validate()
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("validation error = %v, want message containing %q", err, test.message)
			}
		})
	}
}

func TestValidateLimitsEnvironmentOverrideCount(t *testing.T) {
	environment := make(map[string]string, maxEnvironmentOverrides+1)
	for index := 0; index <= maxEnvironmentOverrides; index++ {
		environment["SHHH_TEST_"+strings.Repeat("X", index)] = "value"
	}
	item := Profile{ID: "local", Name: "Local", Protocol: ProtocolLocal, Environment: environment}
	if err := item.Validate(); err == nil || !strings.Contains(err.Error(), "at most 128 overrides") {
		t.Fatalf("environment limit error = %v", err)
	}
}
