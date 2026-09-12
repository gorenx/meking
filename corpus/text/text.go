package text

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/internal/uuid"
)

const MaximumBodyBytes = int(document.MaximumContentBytes)

type ID string

type Format string

const (
	PlainText Format = "plain_text"
	Markdown  Format = "markdown"
)

type Text struct {
	ID                   ID
	DocumentID           document.ID
	Title                string
	Body                 string
	Format               Format
	ExtractionProfile    string
	NormalizationProfile string
	Warnings             []string
}

type Page struct {
	Offset int
	Limit  int
}

type TextPage struct {
	Texts []Text
	Next  *Page
}

func Validate(value Text) error {
	if err := ValidateID(value.ID); err != nil {
		return err
	}
	if err := document.ValidateID(value.DocumentID); err != nil {
		return fmt.Errorf("%w: DocumentID: %v", ErrInvalid, err)
	}
	if value.Title == "" || ValidateBody(value.Body) != nil ||
		(value.Format != PlainText && value.Format != Markdown) ||
		value.ExtractionProfile == "" || value.NormalizationProfile == "" {
		return ErrInvalid
	}
	for _, warning := range value.Warnings {
		if strings.TrimSpace(warning) == "" {
			return fmt.Errorf("%w: warning is empty", ErrInvalid)
		}
	}
	return nil
}

func ValidateBody(body string) error {
	if strings.TrimSpace(body) == "" || !utf8.ValidString(body) ||
		strings.ContainsRune(body, '\x00') || len(body) > MaximumBodyBytes {
		return fmt.Errorf("%w: body is invalid", ErrInvalid)
	}
	return nil
}

func ValidateID(id ID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf("%w: Text ID", ErrInvalid)
	}
	return nil
}
