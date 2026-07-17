package profile

import "testing"

func TestEndpointFormatsIPv6WithoutAmbiguity(t *testing.T) {
	profile := Profile{Protocol: ProtocolSSH, Host: "[2001:db8::1]", Port: 2222, Username: "deploy"}
	if got := profile.Endpoint(); got != "deploy@[2001:db8::1]:2222" {
		t.Fatalf("unexpected endpoint: %q", got)
	}
}
