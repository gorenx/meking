package community

import "context"

type StructureState struct {
	StructureID            StructureID
	PendingRelationChanges uint64
}

func (state StructureState) Validate() error {
	if state.StructureID == "" {
		if state.PendingRelationChanges != 0 {
			return ErrInvalidStructure
		}
		return nil
	}
	if _, err := RestoreStructureID(state.StructureID); err != nil {
		return err
	}
	return nil
}

type StructureStateStore interface {
	LoadStructureState(context.Context) (StructureState, error)
	SaveStructureState(
		ctx context.Context,
		expected StructureState,
		state StructureState,
	) error
}
