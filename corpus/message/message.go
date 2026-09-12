// Package message owns ordered Agent messages backed by immutable TextUnit bodies.
package message

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/memoria-space/meking/corpus/textunits"
)

var (
	ErrInvalid          = errors.New("invalid Message")
	ErrIdentityConflict = errors.New("Message identity conflict")
	ErrSequenceConflict = errors.New("Message sequence conflict")
	ErrNotFound         = errors.New("Message not found")
)

// Message is one immutable Agent message. Its position belongs to an
// Occurrence because the same TextUnit body may appear more than once.
type Message struct {
	ID       string
	Role     string
	TextUnit textunits.TextUnitBody
}

// Occurrence places one Message in its owning Session Zone.
type Occurrence struct {
	Message  Message
	Position uint64
}

func New(id, role, text string) (Message, error) {
	value := Message{ID: id, Role: role}
	if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return Message{}, fmt.Errorf("%w: Text must be valid UTF-8 without NUL", ErrInvalid)
	}
	unit, err := textunits.NewTextUnitBody(text)
	if err != nil {
		return Message{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	value.TextUnit = unit
	if err := Validate(value); err != nil {
		return Message{}, err
	}
	return value, nil
}

func Validate(value Message) error {
	switch {
	case strings.TrimSpace(value.ID) == "":
		return fmt.Errorf("%w: ID is required", ErrInvalid)
	case value.ID != strings.TrimSpace(value.ID):
		return fmt.Errorf("%w: ID must not have surrounding whitespace", ErrInvalid)
	case strings.TrimSpace(value.Role) == "":
		return fmt.Errorf("%w: Role is required", ErrInvalid)
	case value.Role != strings.TrimSpace(value.Role):
		return fmt.Errorf("%w: Role must not have surrounding whitespace", ErrInvalid)
	}
	if err := textunits.ValidateTextUnitBody(value.TextUnit); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

func ValidateOccurrence(value Occurrence) error {
	return Validate(value.Message)
}
