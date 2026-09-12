package memory

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
)

var (
	ErrInvalid       = errors.New("invalid Memory")
	ErrAlreadyExists = errors.New("Memory already exists")
)

type Recorder struct {
	Dependencies
}

func NewRecorder(dependencies Dependencies) (*Recorder, error) {
	switch {
	case dependencies.Messages == nil:
		return nil, errors.New("create Memory Recorder: Messages are required")
	case dependencies.Submissions == nil:
		return nil, errors.New("create Memory Recorder: Knowledge Submissions are required")
	case dependencies.ConflictCatalog == nil:
		return nil, errors.New("create Memory Recorder: Conflict Catalog is required")
	default:
		return &Recorder{Dependencies: dependencies}, nil
	}
}

func (recorder *Recorder) Add(ctx context.Context, memory Memory) (Receipt, error) {
	if recorder == nil {
		return Receipt{}, errors.New("Memory Recorder is not configured")
	}
	if err := memory.Validate(); err != nil {
		return Receipt{}, err
	}
	command, err := knowledgeCommand(memory, evidenceForMessages(memory.Messages))
	if err != nil {
		return Receipt{}, err
	}
	if found, err := recorder.Submissions.HasSource(ctx, memory.ID); err != nil {
		return Receipt{}, fmt.Errorf("check Memory ID: %w", err)
	} else if found {
		return Receipt{}, fmt.Errorf("%w: ID %q", ErrAlreadyExists, memory.ID)
	}
	if err := recorder.Messages.CheckAppend(ctx, memory.Messages); err != nil {
		return Receipt{}, fmt.Errorf("check Memory Messages: %w", err)
	}
	if err := recorder.Submissions.Preflight(ctx, command); err != nil {
		return Receipt{}, fmt.Errorf("check Memory Knowledge: %w", err)
	}
	occurrences, err := recorder.Messages.Append(ctx, memory.Messages)
	if err != nil {
		return Receipt{}, fmt.Errorf("append Memory Messages: %w", err)
	}
	if err := validateOccurrences(memory.Messages, occurrences); err != nil {
		return Receipt{}, err
	}
	command, err = knowledgeCommand(memory, evidenceForOccurrences(occurrences))
	if err != nil {
		return Receipt{}, err
	}
	result, err := recorder.Submissions.Submit(ctx, command)
	if err != nil {
		return Receipt{}, fmt.Errorf("submit Memory Knowledge: %w", err)
	}
	conflicts, err := recorder.conflicts(ctx, result)
	if err != nil {
		return Receipt{}, fmt.Errorf("read Memory conflicts: %w", err)
	}
	return Receipt{
		MemoryID:   memory.ID,
		Messages:   occurrences,
		Submission: result,
		Conflicts:  conflicts,
	}, nil
}

func knowledgeCommand(memory Memory, evidence []provenance.Evidence) (submission.Command, error) {
	command := submission.Command{
		Source:    provenance.Source{ID: memory.ID, Kind: provenance.Agent, ProducerID: memory.ID},
		Entities:  append([]submission.Entity(nil), memory.Knowledge.Entities...),
		Relations: append([]submission.Relation(nil), memory.Knowledge.Relations...),
		Claims:    append([]submission.Claim(nil), memory.Knowledge.Claims...),
	}
	for index := range command.Entities {
		if len(command.Entities[index].Metadata.Evidence) != 0 {
			return submission.Command{}, fmt.Errorf("%w: Entity Evidence is owned by Memory", ErrInvalid)
		}
		command.Entities[index].Metadata.Evidence = append([]provenance.Evidence(nil), evidence...)
	}
	for index := range command.Relations {
		if len(command.Relations[index].Metadata.Evidence) != 0 {
			return submission.Command{}, fmt.Errorf("%w: Relation Evidence is owned by Memory", ErrInvalid)
		}
		command.Relations[index].Metadata.Evidence = append([]provenance.Evidence(nil), evidence...)
	}
	for index := range command.Claims {
		if len(command.Claims[index].Metadata.Evidence) != 0 {
			return submission.Command{}, fmt.Errorf("%w: Claim Evidence is owned by Memory", ErrInvalid)
		}
		command.Claims[index].Metadata.Evidence = append([]provenance.Evidence(nil), evidence...)
	}
	return command, nil
}

func evidenceForMessages(messages []message.Message) []provenance.Evidence {
	evidence := make([]provenance.Evidence, len(messages))
	for index, value := range messages {
		evidence[index] = provenance.Evidence{
			TextUnitID: string(value.TextUnit.ID),
			Source:     provenance.MessageSource{MessageID: value.ID},
		}
	}
	return evidence
}

func evidenceForOccurrences(occurrences []message.Occurrence) []provenance.Evidence {
	messages := make([]message.Message, len(occurrences))
	for index, occurrence := range occurrences {
		messages[index] = occurrence.Message
	}
	return evidenceForMessages(messages)
}

func validateOccurrences(messages []message.Message, occurrences []message.Occurrence) error {
	if len(messages) != len(occurrences) {
		return fmt.Errorf("%w: Corpus returned a different Message count", ErrInvalid)
	}
	for index, occurrence := range occurrences {
		if occurrence.Message != messages[index] {
			return fmt.Errorf("%w: Corpus returned different Message %d", ErrInvalid, index)
		}
	}
	return nil
}

func (recorder *Recorder) conflicts(ctx context.Context, result submission.Result) (ConflictSet, error) {
	conflicts := ConflictSet{
		Entities:  []resolution.EntityConflict{},
		Relations: []resolution.RelationConflict{},
		Claims:    []resolution.ClaimConflict{},
	}
	entityIDs := conflictIDs(result.OpenedCandidates.Entities, result.ReusedCandidates.Entities)
	for _, entityID := range entityIDs {
		value, err := recorder.ConflictCatalog.EntityConflict(ctx, entityID)
		if err != nil {
			return ConflictSet{}, err
		}
		conflicts.Entities = append(conflicts.Entities, value)
	}
	relationIDs := conflictIDs(result.OpenedCandidates.Relations, result.ReusedCandidates.Relations)
	for _, relationID := range relationIDs {
		value, err := recorder.ConflictCatalog.RelationConflict(ctx, relationID)
		if err != nil {
			return ConflictSet{}, err
		}
		conflicts.Relations = append(conflicts.Relations, value)
	}
	claimIDs := conflictIDs(result.OpenedCandidates.Claims, result.ReusedCandidates.Claims)
	for _, claimID := range claimIDs {
		value, err := recorder.ConflictCatalog.ClaimConflict(ctx, claimID)
		if err != nil {
			return ConflictSet{}, err
		}
		conflicts.Claims = append(conflicts.Claims, value)
	}
	return conflicts, nil
}

func conflictIDs[ID knowledge.KnowledgeID](
	opened []candidate.Key[ID],
	reused []candidate.Key[ID],
) []ID {
	ids := make([]ID, 0, len(opened)+len(reused))
	for _, key := range append(append([]candidate.Key[ID](nil), opened...), reused...) {
		ids = append(ids, key.ID)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
