package activation

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
)

type Store interface {
	ArchiveReader
	FindObservation(context.Context, string) (StoredObservation, bool, error)
	ReadState(context.Context, knowledge.ObjectRef) (StoredState, bool, error)
	InsertObservation(context.Context, StoredObservation) error
	SaveState(context.Context, StoredState) error
}

type StoredState struct {
	Subject knowledge.ObjectRef
	State   mas.State
	// Sequence counts applied observations for this Zone and object, not versions.
	Sequence uint64
	ModelID  string
}

func (state StoredState) Validate() error {
	if err := ValidateSubject(state.Subject); err != nil {
		return fmt.Errorf("%w: %v", ErrDataIntegrity, err)
	}
	if state.Sequence == 0 || state.Sequence > math.MaxInt64 || !validText(state.ModelID) || state.State == (mas.State{}) {
		return fmt.Errorf("%w: incomplete memory state", ErrDataIntegrity)
	}
	if err := state.State.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrDataIntegrity, err)
	}
	return nil
}

type Receipt struct {
	ObservationID  string
	Outcome        string
	RecordedAt     time.Time
	ProtocolDigest string
	ModelID        string
	// AppliedSequence is zero when the observation did not change memory state.
	AppliedSequence uint64
	// Duplicate is response-only; the original stored receipt always has false.
	Duplicate bool
}

type StoredObservation struct {
	Input   Observation
	Receipt Receipt
}

func (record StoredObservation) Validate() error {
	input, err := record.Input.Normalize()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDataIntegrity, err)
	}
	receipt := record.Receipt
	if receipt.Duplicate || receipt.ObservationID != input.ID || receipt.ProtocolDigest != input.ProtocolDigest || !validText(receipt.ModelID) || !validTime(receipt.RecordedAt) {
		return fmt.Errorf("%w: invalid observation receipt", ErrDataIntegrity)
	}
	if input.Assessment.Status == "unscorable" {
		if receipt.Outcome != "recorded_unscorable" || receipt.AppliedSequence != 0 {
			return fmt.Errorf("%w: unscorable receipt changes state", ErrDataIntegrity)
		}
		return nil
	}
	if receipt.Outcome != "applied" || receipt.AppliedSequence == 0 || receipt.AppliedSequence > math.MaxInt64 {
		return fmt.Errorf("%w: invalid applied sequence", ErrDataIntegrity)
	}
	return nil
}
