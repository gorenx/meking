package textunits

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/memoria-space/meking/corpus/text"
)

var ErrInvalid = errors.New("invalid TextUnit")

func ValidateTextUnitID(id TextUnitID) error {
	if !isSHA512ID(string(id)) {
		return fmt.Errorf("%w: ID %q is not a lowercase SHA-512 digest", ErrInvalid, id)
	}
	return nil
}

func ValidateTextUnitBody(unit TextUnitBody) error {
	if err := ValidateTextUnitID(unit.ID); err != nil {
		return err
	}
	if !utf8.ValidString(unit.Text) || strings.TrimSpace(unit.Text) == "" {
		return fmt.Errorf("%w: Text must be non-empty UTF-8", ErrInvalid)
	}
	if TextUnitID(sha512Sum(unit.Text)) != unit.ID {
		return fmt.Errorf("%w: content identity does not match Text", ErrInvalid)
	}
	return nil
}

func ValidateTextUnit(unit TextUnit) error {
	if err := ValidateTextUnitBody(unit.TextUnit); err != nil {
		return err
	}
	if unit.TokenCount <= 0 {
		return fmt.Errorf("%w: TokenCount must be positive", ErrInvalid)
	}
	if unit.StartIndex < 0 || unit.EndIndex <= unit.StartIndex {
		return fmt.Errorf("%w: character range is invalid", ErrInvalid)
	}
	return nil
}

func ValidateTextUnitContent(source text.Text, unit TextUnit) error {
	if err := ValidateTextUnit(unit); err != nil {
		return err
	}
	if !utf8.ValidString(source.Body) {
		return fmt.Errorf("%w: source Text must be UTF-8", ErrInvalid)
	}
	runes := []rune(source.Body)
	if unit.EndIndex > len(runes) {
		return fmt.Errorf("%w: character range exceeds source Text", ErrInvalid)
	}
	if string(runes[unit.StartIndex:unit.EndIndex]) != unit.TextUnit.Text {
		return fmt.Errorf("%w: character range does not identify TextUnitBody content", ErrInvalid)
	}
	return nil
}

func sha512Sum(value string) string {
	digest := sha512.Sum512([]byte(value))
	return hex.EncodeToString(digest[:])
}

func isSHA512ID(id string) bool {
	if len(id) != 128 {
		return false
	}
	for _, character := range []byte(id) {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}
