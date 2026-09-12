package text

import "errors"

var (
	ErrInvalid          = errors.New("invalid text")
	ErrNotFound         = errors.New("text not found")
	ErrContentConflict  = errors.New("text content conflict")
	ErrStorageIntegrity = errors.New("text storage integrity failure")
	ErrUnsupported      = errors.New("unsupported source document")
)
