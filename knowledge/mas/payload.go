package mas

import (
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/knowledge"
)

// Grade records observed recall performance. Conversation reactions such as
// citation, confirmation, or objection do not determine a grade by themselves.
type Grade int

const (
	GradeAgain Grade = 1 // Recall failed.
	GradeHard  Grade = 2 // Recall succeeded with substantial difficulty.
	GradeGood  Grade = 3 // Recall succeeded with ordinary effort.
	GradeEasy  Grade = 4 // Recall succeeded easily.
)

func NewGrade(g int) (Grade, error) {
	if g < 1 || g > 4 {
		return 0, errors.New("mas: grade must be in 1..4")
	}
	return Grade(g), nil
}

func (g Grade) Int() int { return int(g) }

// Payload associates a recall result with the exact content being evaluated.
// The caller owns the observation identity, evidence, and Zone scope.
type Payload struct {
	OccuredAt time.Time
	Subject   knowledge.ObjectRef `json:"Subject"`
	Grade     Grade               `json:"Grade"`
	Version   knowledge.Version   `json:"Version"`
}

func (p Payload) Validate() error {
	if _, err := NewGrade(p.Grade.Int()); err != nil {
		return err
	}
	if err := validateTime(p.OccuredAt); err != nil {
		return err
	}
	if p.Version == 0 {
		return errors.New("mas: observed knowledge version must be positive")
	}
	var err error
	switch id := p.Subject.(type) {
	case knowledge.EntityID:
		err = knowledge.ValidateEntityID(id)
	case knowledge.RelationID:
		err = knowledge.ValidateRelationID(id)
	case knowledge.ClaimID:
		err = knowledge.ValidateClaimID(id)
	default:
		return errors.New("mas: subject must be an entity, relation, or claim ID")
	}
	if err != nil {
		return fmt.Errorf("mas: invalid subject: %w", err)
	}
	return nil
}
