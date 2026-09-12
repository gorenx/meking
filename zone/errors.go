package zone

import "errors"

var (
	ErrInvalidDefinition    = errors.New("invalid Zone definition")
	ErrInvalidUser          = errors.New("invalid User identity")
	ErrNotFound             = errors.New("Zone not found")
	ErrNotReady             = errors.New("Zone is not ready")
	ErrContextRequired      = errors.New("Zone context is required")
	ErrChildContextRequired = errors.New("Child Zone context is required")
	ErrContextConflict      = errors.New("Zone context is already bound to another Zone")
	ErrParentRequired       = errors.New("Child Zone requires a direct Parent")
	ErrParentConflict       = errors.New("Zone belongs to another Parent")
	ErrChildDepth           = errors.New("Child Zone cannot own another Child")
)
