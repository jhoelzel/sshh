package apperror

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
)

type Code string

const (
	CodeInvalidArgument        Code = "invalid_argument"
	CodeNotFound               Code = "not_found"
	CodeConflict               Code = "conflict"
	CodeStale                  Code = "stale"
	CodeAuthenticationRequired Code = "authentication_required"
	CodePermissionDenied       Code = "permission_denied"
	CodeUnavailable            Code = "unavailable"
	CodeCanceled               Code = "canceled"
	CodeDeadlineExceeded       Code = "deadline_exceeded"
	CodeInternal               Code = "internal"
)

type Error struct {
	code      Code
	operation string
	message   string
	cause     error
}

type Descriptor struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Operation string `json:"operation,omitempty"`
	Retryable bool   `json:"retryable"`
}

func New(code Code, message string) *Error {
	return &Error{code: normalizeCode(code), message: normalizeMessage(message)}
}

func Wrap(code Code, operation, message string, cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{
		code:      normalizeCode(code),
		operation: operation,
		message:   normalizeMessage(message),
		cause:     cause,
	}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	cause := ""
	if e.cause != nil && e.cause.Error() != e.message {
		cause = ": " + e.cause.Error()
	}
	if e.operation != "" && e.cause != nil {
		return e.operation + ": " + e.message + cause
	}
	if e.operation != "" {
		return e.operation + ": " + e.message
	}
	if cause != "" {
		return e.message + cause
	}
	return e.message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *Error) Code() Code {
	if e == nil {
		return ""
	}
	return e.code
}

func Describe(err error) Descriptor {
	if err == nil {
		return Descriptor{}
	}

	var typed *Error
	if errors.As(err, &typed) {
		return Descriptor{
			Code:      typed.code,
			Message:   typed.message,
			Operation: typed.operation,
			Retryable: retryable(typed.code),
		}
	}

	switch {
	case errors.Is(err, context.Canceled):
		return descriptor(CodeCanceled, "Operation was canceled.")
	case errors.Is(err, context.DeadlineExceeded):
		return descriptor(CodeDeadlineExceeded, "Operation timed out.")
	case errors.Is(err, fs.ErrNotExist):
		return descriptor(CodeNotFound, "Requested file or resource was not found.")
	case errors.Is(err, fs.ErrPermission):
		return descriptor(CodePermissionDenied, "Permission was denied.")
	}

	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return descriptor(CodeDeadlineExceeded, "Operation timed out.")
	}
	return descriptor(CodeInternal, err.Error())
}

func CodeOf(err error) Code {
	return Describe(err).Code
}

func IsCode(err error, code Code) bool {
	return err != nil && CodeOf(err) == code
}

// Format is compatible with Wails' ErrorFormatter and intentionally returns a
// JSON string because the Wails v2 runtime converts rejected values to Error.
func Format(err error) any {
	encoded, marshalErr := json.Marshal(Describe(err))
	if marshalErr != nil {
		return `{"code":"internal","message":"The operation could not be completed.","retryable":false}`
	}
	return string(encoded)
}

func descriptor(code Code, message string) Descriptor {
	code = normalizeCode(code)
	return Descriptor{Code: code, Message: normalizeMessage(message), Retryable: retryable(code)}
}

func retryable(code Code) bool {
	return code == CodeStale || code == CodeUnavailable || code == CodeDeadlineExceeded
}

func normalizeCode(code Code) Code {
	switch code {
	case CodeInvalidArgument, CodeNotFound, CodeConflict, CodeStale,
		CodeAuthenticationRequired, CodePermissionDenied, CodeUnavailable,
		CodeCanceled, CodeDeadlineExceeded, CodeInternal:
		return code
	default:
		return CodeInternal
	}
}

func normalizeMessage(message string) string {
	if message == "" {
		return "The operation could not be completed."
	}
	return message
}
