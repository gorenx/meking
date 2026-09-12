package mas

import (
	"errors"
	"time"

	"github.com/memoria-space/meking/knowledge"
)

// ErrStateNotFound distinguishes missing history from a failed state read.
var ErrStateNotFound = errors.New("mas: state not found")

type Snapshot struct {
	Subject    knowledge.ObjectRef
	Stability  Stability
	Difficulty Difficulty
	LastReview time.Time
	UpdatedAt  time.Time
}
