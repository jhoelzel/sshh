package snippet

import (
	"strings"
	"testing"
)

func TestVariablesAndRenderPreserveExactPreview(t *testing.T) {
	body := "deploy --host {{ host }} --tag '{{tag}}' && echo {{host}}"
	variables, err := Variables(body)
	if err != nil {
		t.Fatalf("variables: %v", err)
	}
	if strings.Join(variables, ",") != "host,tag" {
		t.Fatalf("unexpected variables: %#v", variables)
	}
	rendered, err := Render(body, map[string]string{"host": "db-1", "tag": "release candidate"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if rendered != "deploy --host db-1 --tag 'release candidate' && echo db-1" {
		t.Fatalf("unexpected rendering %q", rendered)
	}
}

func TestRenderRejectsMissingUnknownAndMalformedVariables(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		values map[string]string
	}{
		{name: "missing", body: "echo {{value}}", values: map[string]string{}},
		{name: "unknown", body: "echo ok", values: map[string]string{"other": "x"}},
		{name: "unclosed", body: "echo {{value", values: map[string]string{}},
		{name: "closing", body: "echo value}}", values: map[string]string{}},
		{name: "invalid", body: "echo {{bad-name}}", values: map[string]string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Render(test.body, test.values); err == nil {
				t.Fatal("expected render error")
			}
		})
	}
}
