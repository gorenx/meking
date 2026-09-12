package journal

import "errors"

var (
	ErrInvalidEvent        = errors.New("invalid Journal event")
	ErrEventConflict       = errors.New("Journal event identity conflict")
	ErrInvalidRegistration = errors.New("invalid Journal registration")
	ErrConsumerPosition    = errors.New("invalid Journal Consumer Position")
)
