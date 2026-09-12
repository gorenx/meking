package document

import "errors"

var (
	ErrInvalid          = errors.New("invalid source document")
	ErrNotFound         = errors.New("source document not found")
	ErrContentTooLarge  = errors.New("source document content is too large")
	ErrContentConflict  = errors.New("source document content conflict")
	ErrStorageIntegrity = errors.New("source document storage integrity failure")
)
