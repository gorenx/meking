// Package document owns immutable source file records and verified content.
package document

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"strings"
	"unicode"
	"unicode/utf8"
)

const idPrefix = "doc_"

type ID string

// Document identifies one immutable source input. Text extraction and
// normalization belong to text and never mutate this value.
type Document struct {
	ID        ID
	Name      string
	MediaType string
	Size      int64
	Digest    string
}

func New(
	name string,
	mediaType string,
	data []byte,
) (Document, error) {
	if len(data) == 0 {
		return Document{}, fmt.Errorf("%w: content is required", ErrInvalid)
	}
	digest := sha256.Sum256(data)
	return newDocument(name, mediaType, int64(len(data)), hex.EncodeToString(digest[:]))
}

func newDocument(name, mediaType string, size int64, digest string) (Document, error) {
	value := Document{
		Name:      strings.TrimSpace(name),
		MediaType: strings.TrimSpace(mediaType),
		Size:      size,
		Digest:    digest,
	}
	id, err := newID()
	if err != nil {
		return Document{}, err
	}
	value.ID = id
	restored, err := Restore(value)
	if err != nil {
		return Document{}, err
	}
	return restored, nil
}

func Restore(value Document) (Document, error) {
	value.Name = strings.TrimSpace(value.Name)
	value.MediaType = strings.TrimSpace(value.MediaType)
	if err := ValidateID(value.ID); err != nil {
		return Document{}, err
	}
	if !validText(value.Name, 1024) {
		return Document{}, fmt.Errorf("%w: name is invalid", ErrInvalid)
	}
	parsedMediaType, _, err := mime.ParseMediaType(value.MediaType)
	if err != nil || parsedMediaType != value.MediaType {
		return Document{}, fmt.Errorf("%w: media type is invalid", ErrInvalid)
	}
	if value.Size <= 0 {
		return Document{}, fmt.Errorf("%w: size must be positive", ErrInvalid)
	}
	if !isSHA256(value.Digest) {
		return Document{}, fmt.Errorf("%w: digest must be a lowercase SHA-256 value", ErrInvalid)
	}
	return value, nil
}

func VerifyContent(value Document, data []byte) error {
	digest := sha256.Sum256(data)
	return verifyContent(value, int64(len(data)), hex.EncodeToString(digest[:]))
}

func verifyContent(value Document, size int64, digest string) error {
	if size != value.Size {
		return fmt.Errorf("%w: size does not match metadata", ErrStorageIntegrity)
	}
	if digest != value.Digest {
		return fmt.Errorf("%w: digest does not match metadata", ErrStorageIntegrity)
	}
	return nil
}

func newID() (ID, error) {
	raw := make([]byte, sha256.Size)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("create Document ID: %w", err)
	}
	return ID(idPrefix + hex.EncodeToString(raw)), nil
}

func ValidateID(id ID) error {
	raw := strings.TrimPrefix(string(id), idPrefix)
	if len(raw) != sha256.Size*2 || idPrefix+raw != string(id) {
		return fmt.Errorf("%w: Document ID", ErrInvalid)
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return fmt.Errorf("%w: Document ID", ErrInvalid)
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range []byte(value) {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}

func validText(value string, maximumRunes int) bool {
	if value == "" || strings.TrimSpace(value) == "" || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maximumRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
