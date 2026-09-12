package activation

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/transaction"
	"github.com/memoria-space/meking/zone"
)

type ArchiveReader interface {
	ReadModel(context.Context, string) (Model, bool, error)
	ReadProtocol(context.Context, string) (Protocol, bool, error)
}

type PreparationStore interface {
	ArchiveReader
	InsertModel(context.Context, Model) error
	InsertProtocol(context.Context, Protocol) error
	// Applied observations for each Zone/object must be in ascending application
	// sequence; unscorable records may appear anywhere in the result.
	ListObservations(context.Context) ([]ScopedObservation, error)
	ListStates(context.Context) ([]ScopedState, error)
}

// Scoped records are startup read projections. Normal submission scope still
// comes exclusively from context, never from a caller-supplied record field.
type ScopedObservation struct {
	ZoneID      zone.ID
	Observation StoredObservation
}
type ScopedState struct {
	ZoneID zone.ID
	State  StoredState
}

// Prepare verifies persisted history before exposing the application and then
// records the immutable startup configuration. SQLite does not own these rules.
func Prepare(ctx context.Context, tx transaction.Tx, store PreparationStore, model Model, protocols []Protocol) error {
	if tx == nil || store == nil || model.scheduler == nil || len(protocols) == 0 {
		return fmt.Errorf("activation: preparation requires transaction, store, model and protocols")
	}
	return tx.WithTx(ctx, func(ctx context.Context) error {
		if err := Verify(ctx, store); err != nil {
			return err
		}
		existing, found, err := store.ReadModel(ctx, model.ID())
		if err != nil {
			return err
		}
		if found {
			if existing.ID() != model.ID() || existing.Config() != model.Config() {
				return ErrDataIntegrity
			}
		} else if err := store.InsertModel(ctx, model); err != nil {
			return err
		}
		languages := map[string]bool{}
		for _, protocol := range protocols {
			if protocol.Digest() == "" || languages[protocol.Language()] {
				return ErrProtocolUnsupported
			}
			languages[protocol.Language()] = true
			existing, found, err := store.ReadProtocol(ctx, protocol.Digest())
			if err != nil {
				return err
			}
			if found {
				if existing != protocol {
					return ErrDataIntegrity
				}
			} else if err := store.InsertProtocol(ctx, protocol); err != nil {
				return err
			}
		}
		return nil
	})
}

// Verify checks all stored Zones against original observations and archived
// models. Call it inside a read-consistent transaction, as Prepare does.
func Verify(ctx context.Context, store PreparationStore) error {
	if store == nil {
		return fmt.Errorf("activation: verification store is required")
	}
	observations, err := store.ListObservations(ctx)
	if err != nil {
		return err
	}
	states, err := store.ListStates(ctx)
	if err != nil {
		return err
	}
	type key struct {
		zone    zone.ID
		subject knowledge.ObjectRef
	}
	histories := map[key][]StoredObservation{}
	protocols := map[string]Protocol{}
	models := map[string]Model{}
	for _, scoped := range observations {
		if _, err := zone.ParseID(string(scoped.ZoneID)); err != nil {
			return fmt.Errorf("%w: invalid observation Zone", ErrDataIntegrity)
		}
		record := scoped.Observation
		if err := record.Validate(); err != nil {
			return err
		}
		protocol, found := protocols[record.Input.ProtocolDigest]
		if !found {
			var err error
			protocol, found, err = store.ReadProtocol(ctx, record.Input.ProtocolDigest)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("%w: missing observation protocol", ErrDataIntegrity)
			}
			protocols[record.Input.ProtocolDigest] = protocol
		}
		if protocol.Digest() != record.Input.ProtocolDigest || protocol.Version() != record.Input.ProtocolVersion {
			return fmt.Errorf("%w: mismatched observation protocol", ErrDataIntegrity)
		}
		if _, found := models[record.Receipt.ModelID]; !found {
			model, found, err := store.ReadModel(ctx, record.Receipt.ModelID)
			if err != nil {
				return err
			}
			if !found || model.ID() != record.Receipt.ModelID {
				return fmt.Errorf("%w: missing observation model", ErrDataIntegrity)
			}
			models[model.ID()] = model
		}
		if record.Receipt.AppliedSequence > 0 {
			k := key{scoped.ZoneID, record.Input.Subject}
			histories[k] = append(histories[k], record)
		}
	}
	for _, scoped := range states {
		if _, err := zone.ParseID(string(scoped.ZoneID)); err != nil {
			return fmt.Errorf("%w: invalid state Zone", ErrDataIntegrity)
		}
		state := scoped.State
		if err := state.Validate(); err != nil {
			return err
		}
		k := key{scoped.ZoneID, state.Subject}
		history, found := histories[k]
		if !found {
			return fmt.Errorf("%w: state has no applied history", ErrDataIntegrity)
		}
		model, found := models[state.ModelID]
		if !found {
			return fmt.Errorf("%w: missing state model", ErrDataIntegrity)
		}
		replayed, found, err := Replay(model, history)
		if err != nil {
			return err
		}
		if !found || replayed != state {
			return fmt.Errorf("%w: current state differs from replay", ErrDataIntegrity)
		}
		delete(histories, k)
	}
	if len(histories) != 0 {
		return fmt.Errorf("%w: applied history has no state", ErrDataIntegrity)
	}
	return nil
}
