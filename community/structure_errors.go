package community

import "errors"

var (
	ErrInvalidStructure  = errors.New("invalid Community Structure")
	ErrStructureNotFound = errors.New("Community Structure not found")
	ErrStructureConflict = errors.New("Community Structure conflicts with committed state")
)
