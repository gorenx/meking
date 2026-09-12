package submission

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/zone"
)

type transactionState struct {
	ctx       context.Context
	sourceID  string
	result    Result
	entities  map[knowledge.EntityIdentity]entityState
	relations map[string]relationState
}

type entityState struct {
	id     knowledge.EntityID
	active bool
}

type relationState struct {
	id     knowledge.RelationID
	active bool
}

func New(dependencies Dependencies) (*Application, error) {
	switch {
	case dependencies.Tx == nil:
		return nil, errors.New("create Knowledge Submission: transaction is required")
	case dependencies.KnowledgeIdentities == nil:
		return nil, errors.New("create Knowledge Submission: Knowledge identities are required")
	case dependencies.VersionHistory == nil:
		return nil, errors.New("create Knowledge Submission: Knowledge versions are required")
	case dependencies.Sources == nil:
		return nil, errors.New("create Knowledge Submission: Sources are required")
	case dependencies.Candidates == nil:
		return nil, errors.New("create Knowledge Submission: Candidates are required")
	case dependencies.EvidenceVerifier == nil:
		return nil, errors.New("create Knowledge Submission: Evidence verifier is required")
	}
	return &Application{Dependencies: dependencies}, nil
}

func (application *Application) Submit(ctx context.Context, command Command) (Result, error) {
	if application == nil {
		return Result{}, errors.New("Knowledge Submission is not configured")
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return Result{}, err
	}
	batch, err := normalizeCommand(bindEvidenceZone(command, string(zoneID)))
	if err != nil {
		return Result{}, fmt.Errorf("submit Knowledge: %w", err)
	}
	if result, found, err := application.acceptedResult(ctx, batch.source); err != nil || found {
		return result, err
	}
	if err := application.EvidenceVerifier.VerifyEvidence(ctx, batch.evidence); err != nil {
		return Result{}, fmt.Errorf("verify Knowledge Evidence: %w", err)
	}

	var result Result
	err = application.Tx.WithTx(ctx, func(ctx context.Context) error {
		if stored, found, err := application.Sources.Source(ctx, batch.source.ID); err != nil {
			return err
		} else if found {
			if stored != batch.source {
				return provenance.SourceIDConflict(batch.source.ID)
			}
			result, err = application.Sources.SubmissionResult(ctx, batch.source.ID)
			return err
		}

		if err = application.Sources.RecordSource(ctx, batch.source, batch.evidence); err != nil {
			return err
		}
		state := transactionState{
			ctx:       ctx,
			sourceID:  batch.source.ID,
			result:    Result{SourceID: batch.source.ID},
			entities:  make(map[knowledge.EntityIdentity]entityState, len(batch.entities)),
			relations: make(map[string]relationState, len(batch.relations)),
		}
		if err := application.submitEntities(&state, batch.entities); err != nil {
			return err
		}
		if err := application.submitRelations(&state, batch.relations); err != nil {
			return err
		}
		if err := application.submitClaims(&state, batch.claims); err != nil {
			return err
		}
		result = state.result
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("submit Knowledge: %w", err)
	}
	return result, nil
}

// HasSource reports whether a Source identity has already entered Knowledge
// without exposing the stored provenance record.
func (application *Application) HasSource(ctx context.Context, sourceID string) (bool, error) {
	if application == nil {
		return false, errors.New("Knowledge Submission is not configured")
	}
	_, found, err := application.Sources.Source(ctx, sourceID)
	return found, err
}

func bindEvidenceZone(command Command, zoneID string) Command {
	command.Entities = append([]Entity(nil), command.Entities...)
	for index := range command.Entities {
		command.Entities[index].Metadata.Evidence = bindEvidence(
			command.Entities[index].Metadata.Evidence,
			zoneID,
		)
	}
	command.Relations = append([]Relation(nil), command.Relations...)
	for index := range command.Relations {
		command.Relations[index].Metadata.Evidence = bindEvidence(
			command.Relations[index].Metadata.Evidence,
			zoneID,
		)
	}
	command.Claims = append([]Claim(nil), command.Claims...)
	for index := range command.Claims {
		command.Claims[index].Metadata.Evidence = bindEvidence(
			command.Claims[index].Metadata.Evidence,
			zoneID,
		)
	}
	return command
}

func bindEvidence(evidence []provenance.Evidence, zoneID string) []provenance.Evidence {
	bound := append([]provenance.Evidence(nil), evidence...)
	for index := range bound {
		if bound[index].ZoneID == "" {
			bound[index].ZoneID = zoneID
		}
	}
	return bound
}

func (application *Application) acceptedResult(
	ctx context.Context,
	source provenance.Source,
) (Result, bool, error) {
	stored, found, err := application.Sources.Source(ctx, source.ID)
	if err != nil || !found {
		return Result{}, false, err
	}
	if stored != source {
		return Result{}, true, provenance.SourceIDConflict(source.ID)
	}
	result, err := application.Sources.SubmissionResult(ctx, source.ID)
	return result, true, err
}
