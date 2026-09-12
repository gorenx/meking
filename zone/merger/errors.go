package merger

import "errors"

var (
	ErrNotReady        = errors.New("Zone Merger is not ready")
	ErrInvalidInput    = errors.New("invalid Zone merge input")
	ErrNotDirectParent = errors.New("current Zone is not the Child Zone direct Parent")
)
