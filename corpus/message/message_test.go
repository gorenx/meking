package message

import (
	"errors"
	"testing"
)

func TestNewBuildsContentIdentity(t *testing.T) {
	first, err := New("message-1", "user", "same text")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	second, err := New("message-2", "assistant", "same text")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if first.TextUnit.ID != second.TextUnit.ID {
		t.Fatalf("TextUnit IDs differ: %q != %q", first.TextUnit.ID, second.TextUnit.ID)
	}
}

func TestNewRejectsInvalidMessage(t *testing.T) {
	for _, test := range []struct {
		name string
		id   string
		role string
		text string
	}{
		{name: "missing ID", role: "user", text: "text"},
		{name: "missing Role", id: "message-1", text: "text"},
		{name: "missing Text", id: "message-1", role: "user"},
		{name: "NUL Text", id: "message-1", role: "user", text: "bad\x00text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.id, test.role, test.text); !errors.Is(err, ErrInvalid) {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}
