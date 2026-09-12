package activation

import (
	"fmt"

	"github.com/memoria-space/meking/knowledge/mas"
)

// Replay reconstructs memory from the archived model and application order.
// It neither calls an evaluator nor reads current Knowledge.
func Replay(model Model, observations []StoredObservation) (StoredState, bool, error) {
	if model.scheduler == nil {
		return StoredState{}, false, ErrModelMismatch
	}
	var state StoredState
	for index, observation := range observations {
		if err := observation.Validate(); err != nil {
			return StoredState{}, false, err
		}
		if observation.Receipt.AppliedSequence != uint64(index)+1 {
			return StoredState{}, false, fmt.Errorf("%w: incomplete application order", ErrDataIntegrity)
		}
		if observation.Receipt.ModelID != model.ID() {
			return StoredState{}, false, ErrModelMismatch
		}
		if index > 0 && observation.Input.Subject != state.Subject {
			return StoredState{}, false, fmt.Errorf("%w: replay mixes subjects", ErrDataIntegrity)
		}
		previous := mas.State{}
		if index > 0 {
			previous = state.State
		}
		next, err := model.scheduler.UpdateLiveness(previous, *observation.Input.Assessment.Grade, observation.Input.OccurredAt)
		if err != nil {
			return StoredState{}, false, fmt.Errorf("%w: replay failed: %v", ErrDataIntegrity, err)
		}
		state = StoredState{Subject: observation.Input.Subject, State: next, Sequence: observation.Receipt.AppliedSequence, ModelID: model.ID()}
	}
	return state, len(observations) > 0, nil
}
