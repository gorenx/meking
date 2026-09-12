package activation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/transaction"
	"github.com/memoria-space/meking/zone"
)

type CurrentTarget struct {
	Version knowledge.Version
	Deleted bool
}
type CurrentTargets interface {
	ReadCurrent(context.Context, knowledge.ObjectRef) (CurrentTarget, bool, error)
}
type EvidenceReader interface {
	Validate(context.Context, []Evidence) error
}

type Dependencies struct {
	Tx        transaction.Tx
	Store     Store
	Targets   CurrentTargets
	Evidence  EvidenceReader
	Protocols []Protocol
	Model     Model
	Now       func() time.Time
}

type Service struct {
	tx        transaction.Tx
	store     Store
	targets   CurrentTargets
	evidence  EvidenceReader
	protocols map[string]Protocol
	model     Model
	now       func() time.Time
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.Tx == nil || deps.Store == nil || deps.Targets == nil || deps.Evidence == nil || deps.Now == nil || deps.Model.scheduler == nil || len(deps.Protocols) == 0 {
		return nil, errors.New("activation: transaction, store, current targets, evidence, protocols, model and clock are required")
	}
	protocols := make(map[string]Protocol, len(deps.Protocols))
	languages := make(map[string]bool, len(deps.Protocols))
	for _, protocol := range deps.Protocols {
		if protocol.Digest() == "" {
			return nil, ErrProtocolUnsupported
		}
		if _, exists := protocols[protocol.Digest()]; exists {
			return nil, errors.New("activation: duplicate evaluation protocol")
		}
		if languages[protocol.Language()] {
			return nil, errors.New("activation: only one current protocol per language is allowed")
		}
		languages[protocol.Language()] = true
		protocols[protocol.Digest()] = protocol
	}
	return &Service{tx: deps.Tx, store: deps.Store, targets: deps.Targets, evidence: deps.Evidence, protocols: protocols, model: deps.Model, now: deps.Now}, nil
}

// Protocol returns the immutable current evaluation protocol for a language.
func (service *Service) Protocol(language string) (Protocol, bool) {
	for _, protocol := range service.protocols {
		if protocol.Language() == language {
			return protocol, true
		}
	}
	return Protocol{}, false
}

func (service *Service) Submit(ctx context.Context, input Observation) (Receipt, error) {
	if _, err := zone.RequireID(ctx); err != nil {
		return Receipt{}, err
	}
	input, err := input.Normalize()
	if err != nil {
		return Receipt{}, err
	}
	if receipt, found, err := service.duplicate(ctx, input); found || err != nil {
		return receipt, err
	}
	if err := service.validateNew(ctx, input); err != nil {
		// A concurrent identical request may have committed while validation was
		// reading another context. A committed receipt remains authoritative.
		if receipt, found, lookupErr := service.duplicate(ctx, input); found || lookupErr != nil {
			return receipt, lookupErr
		}
		return Receipt{}, err
	}
	var receipt Receipt
	err = service.tx.WithTx(ctx, func(ctx context.Context) error {
		if original, found, err := service.duplicate(ctx, input); found || err != nil {
			receipt = original
			return err
		}
		protocol, found, err := service.store.ReadProtocol(ctx, input.ProtocolDigest)
		if err != nil {
			return err
		}
		if !found || protocol != service.protocols[input.ProtocolDigest] {
			return fmt.Errorf("%w: observation protocol is not archived", ErrDataIntegrity)
		}
		model, found, err := service.store.ReadModel(ctx, service.model.ID())
		if err != nil {
			return err
		}
		if !found || model.ID() != service.model.ID() || model.Config() != service.model.Config() {
			return fmt.Errorf("%w: observation model is not archived", ErrDataIntegrity)
		}
		at := service.now().UTC()
		if !validTime(at) {
			return errors.New("activation: clock returned an invalid receipt time")
		}
		receipt = Receipt{ObservationID: input.ID, Outcome: "recorded_unscorable", RecordedAt: at, ProtocolDigest: input.ProtocolDigest, ModelID: service.model.ID()}
		var applied *StoredState
		if input.Assessment.Status == "graded" {
			stored, found, err := service.store.ReadState(ctx, input.Subject)
			if err != nil {
				return err
			}
			state := mas.State{}
			sequence := uint64(1)
			if found {
				if err := stored.Validate(); err != nil {
					return err
				}
				if stored.Subject != input.Subject {
					return ErrDataIntegrity
				}
				if stored.ModelID != service.model.ID() {
					return ErrModelMismatch
				}
				if stored.Sequence == math.MaxInt64 {
					return fmt.Errorf("%w: applied sequence exhausted", ErrDataIntegrity)
				}
				state, sequence = stored.State, stored.Sequence+1
			}
			if !state.LastReview.IsZero() && input.OccurredAt.Before(state.LastReview) {
				return ErrOutOfOrder
			}
			next, err := service.model.scheduler.UpdateLiveness(state, *input.Assessment.Grade, input.OccurredAt)
			if err != nil {
				return err
			}
			receipt.Outcome = "applied"
			receipt.AppliedSequence = sequence
			applied = &StoredState{Subject: input.Subject, State: next, Sequence: sequence, ModelID: service.model.ID()}
		}
		record := StoredObservation{Input: input, Receipt: receipt}
		if err := record.Validate(); err != nil {
			return err
		}
		if err := service.store.InsertObservation(ctx, record); err != nil {
			return err
		}
		if applied != nil {
			return service.store.SaveState(ctx, *applied)
		}
		return nil
	})
	if err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func (service *Service) validateNew(ctx context.Context, input Observation) error {
	protocol, found := service.protocols[input.ProtocolDigest]
	if !found || protocol.Version() != input.ProtocolVersion {
		return ErrProtocolUnsupported
	}
	if err := service.evidence.Validate(ctx, input.Evidence); err != nil {
		return err
	}
	target, found, err := service.targets.ReadCurrent(ctx, input.Subject)
	if err != nil {
		return err
	}
	if !found || target.Deleted {
		return ErrTargetNotFound
	}
	if target.Version != input.Version {
		return ErrVersionChanged
	}
	return nil
}

func (service *Service) duplicate(ctx context.Context, input Observation) (Receipt, bool, error) {
	record, found, err := service.store.FindObservation(ctx, input.ID)
	if err != nil || !found {
		return Receipt{}, false, err
	}
	if err := record.Validate(); err != nil {
		return Receipt{}, false, err
	}
	original, err := record.Input.Normalize()
	if err != nil {
		return Receipt{}, false, err
	}
	if !reflect.DeepEqual(original, input) {
		return Receipt{}, false, ErrObservationConflict
	}
	receipt := record.Receipt
	receipt.Duplicate = true
	return receipt, true, nil
}

func (service *Service) ReadState(ctx context.Context, subject knowledge.ObjectRef) (StoredState, bool, error) {
	if _, err := zone.RequireID(ctx); err != nil {
		return StoredState{}, false, err
	}
	if err := ValidateSubject(subject); err != nil {
		return StoredState{}, false, err
	}
	state, found, err := service.store.ReadState(ctx, subject)
	if err != nil || !found {
		return StoredState{}, found, err
	}
	if err := state.Validate(); err != nil {
		return StoredState{}, false, err
	}
	if state.Subject != subject {
		return StoredState{}, false, ErrDataIntegrity
	}
	return state, true, nil
}
