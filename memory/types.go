// Package memory coordinates Agent memories across Corpus and Knowledge.
package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/memoria-space/meking/corpus/message"
	knowledgedomain "github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
)

// Memory is one immutable Agent submission and its supporting Messages.
type Memory struct {
	ID        string
	Messages  []message.Message
	Knowledge Knowledge
}

func (memory Memory) Validate() error {
	if strings.TrimSpace(memory.ID) == "" || memory.ID != strings.TrimSpace(memory.ID) {
		return fmt.Errorf("%w: ID is required without surrounding whitespace", ErrInvalid)
	}
	if len(memory.Messages) == 0 {
		return fmt.Errorf("%w: Messages are required", ErrInvalid)
	}
	if len(memory.Knowledge.Entities) == 0 &&
		len(memory.Knowledge.Relations) == 0 &&
		len(memory.Knowledge.Claims) == 0 {
		return fmt.Errorf("%w: Knowledge must contain at least one Entity, Relation, or Claim", ErrInvalid)
	}
	for index, value := range memory.Messages {
		if err := message.Validate(value); err != nil {
			return fmt.Errorf("%w: Message %d: %v", ErrInvalid, index, err)
		}
	}
	return nil
}

type Knowledge struct {
	Entities  []submission.Entity
	Relations []submission.Relation
	Claims    []submission.Claim
}

type ConflictSet struct {
	Entities  []resolution.EntityConflict
	Relations []resolution.RelationConflict
	Claims    []resolution.ClaimConflict
}

type Receipt struct {
	MemoryID   string
	Messages   []message.Occurrence
	Submission submission.Result
	Conflicts  ConflictSet
}

type Messages interface {
	CheckAppend(ctx context.Context, messages []message.Message) error
	Append(ctx context.Context, messages []message.Message) ([]message.Occurrence, error)
}

type Submissions interface {
	HasSource(ctx context.Context, sourceID string) (bool, error)
	Preflight(ctx context.Context, command submission.Command) error
	Submit(ctx context.Context, command submission.Command) (submission.Result, error)
}

type ConflictCatalog interface {
	EntityConflict(ctx context.Context, entityID knowledgedomain.EntityID) (resolution.EntityConflict, error)
	RelationConflict(ctx context.Context, relationID knowledgedomain.RelationID) (resolution.RelationConflict, error)
	ClaimConflict(ctx context.Context, claimID knowledgedomain.ClaimID) (resolution.ClaimConflict, error)
}

type Dependencies struct {
	Messages        Messages
	Submissions     Submissions
	ConflictCatalog ConflictCatalog
}
