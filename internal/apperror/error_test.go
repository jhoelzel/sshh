package apperror

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestDescribePreservesTypedCodeAndSafeMessage(t *testing.T) {
	t.Parallel()

	cause := errors.New("secret internal detail")
	err := Wrap(CodeUnavailable, "open terminal", "The SSH service is unavailable.", cause)
	descriptor := Describe(err)

	if descriptor.Code != CodeUnavailable || descriptor.Message != "The SSH service is unavailable." {
		t.Fatalf("unexpected descriptor: %#v", descriptor)
	}
	if descriptor.Operation != "open terminal" || !descriptor.Retryable {
		t.Fatalf("missing operation metadata: %#v", descriptor)
	}
	if !errors.Is(err, cause) {
		t.Fatal("typed error no longer unwraps its cause")
	}
}

func TestDescribeClassifiesStandardErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code Code
	}{
		{name: "canceled", err: context.Canceled, code: CodeCanceled},
		{name: "deadline", err: context.DeadlineExceeded, code: CodeDeadlineExceeded},
		{name: "not found", err: fs.ErrNotExist, code: CodeNotFound},
		{name: "permission", err: fs.ErrPermission, code: CodePermissionDenied},
		{name: "ordinary", err: errors.New("unexpected"), code: CodeInternal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CodeOf(test.err); got != test.code {
				t.Fatalf("CodeOf() = %q, want %q", got, test.code)
			}
		})
	}
}

func TestFormatProducesFrontendEnvelopeWithoutCause(t *testing.T) {
	t.Parallel()

	formatted, ok := Format(Wrap(
		CodeConflict,
		"save settings",
		"Settings changed outside shh-h; reload before saving.",
		errors.New("/private/path/settings.json"),
	)).(string)
	if !ok {
		t.Fatalf("Format() returned %T, want string", formatted)
	}
	if strings.Contains(formatted, "/private/path") {
		t.Fatalf("formatted error leaked its cause: %s", formatted)
	}

	var descriptor Descriptor
	if err := json.Unmarshal([]byte(formatted), &descriptor); err != nil {
		t.Fatalf("decode formatted error: %v", err)
	}
	if descriptor.Code != CodeConflict || descriptor.Retryable {
		t.Fatalf("unexpected formatted descriptor: %#v", descriptor)
	}
}

func TestStandardFilesystemClassificationDoesNotExposePath(t *testing.T) {
	t.Parallel()

	descriptor := Describe(errors.Join(fs.ErrNotExist, errors.New("/private/project/config.json")))
	if descriptor.Code != CodeNotFound {
		t.Fatalf("filesystem error code = %q, want %q", descriptor.Code, CodeNotFound)
	}
	if strings.Contains(descriptor.Message, "/private/project") {
		t.Fatalf("filesystem descriptor leaked its path: %q", descriptor.Message)
	}
}

func TestInvalidCodeFallsBackToInternal(t *testing.T) {
	t.Parallel()

	if got := New(Code("made_up"), "failure").Code(); got != CodeInternal {
		t.Fatalf("invalid code normalized to %q", got)
	}
}
