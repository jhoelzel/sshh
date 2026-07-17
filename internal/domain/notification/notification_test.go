package notification

import (
	"strings"
	"testing"
)

func TestMessageValidation(t *testing.T) {
	if err := (Message{ID: "transfer-one", Title: "Transfer completed", Body: "Finished"}).Validate(); err != nil {
		t.Fatalf("valid message rejected: %v", err)
	}
	for _, message := range []Message{
		{Title: "Missing ID"},
		{ID: "id", Title: ""},
		{ID: "id", Title: strings.Repeat("x", maxTitleLength+1)},
		{ID: "id", Title: "Unsafe", Body: "bad\x00body"},
		{ID: "id", Title: "Unsafe", Body: "bad\nbody"},
	} {
		if err := message.Validate(); err == nil {
			t.Fatalf("invalid message accepted: %#v", message)
		}
	}
}
