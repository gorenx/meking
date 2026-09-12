package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
)

func TestRecorderReusesMessagesAfterKnowledgeSubmissionFails(t *testing.T) {
	messages := &failingSubmissionMessages{}
	submissions := &failingSubmission{nextErr: knowledge.ErrStorageBusy}
	recorder, err := NewRecorder(Dependencies{
		Messages:        messages,
		Submissions:     submissions,
		ConflictCatalog: emptyConflictCatalog{},
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := message.New("message-1", "user", "Alice is an engineer.")
	if err != nil {
		t.Fatal(err)
	}
	memory := Memory{
		ID:       "memory-1",
		Messages: []message.Message{value},
		Knowledge: Knowledge{Entities: []submission.Entity{{
			Content: knowledge.EntityContent{
				Identity:    knowledge.EntityIdentity{Title: "ALICE", Type: "PERSON"},
				Description: "Alice is an engineer.",
			},
			Metadata: provenance.EntityMetadata{Frequency: 1},
		}}},
	}

	if _, err := recorder.Add(t.Context(), memory); !errors.Is(err, knowledge.ErrStorageBusy) {
		t.Fatalf("first Add error = %v", err)
	}
	if len(messages.saved) != 1 || messages.saved[0].Message != value {
		t.Fatalf("saved Messages = %#v", messages.saved)
	}
	if _, err := recorder.Add(t.Context(), memory); err != nil {
		t.Fatalf("second Add error = %v", err)
	}
	if submissions.submitCalls != 2 || len(messages.saved) != 1 {
		t.Fatalf("calls after retry = submit %d, Messages %#v", submissions.submitCalls, messages.saved)
	}
}

func TestMemoryRequiresKnowledge(t *testing.T) {
	value, err := message.New("message-1", "user", "Alice is an engineer.")
	if err != nil {
		t.Fatal(err)
	}
	memory := Memory{
		ID:       "memory-1",
		Messages: []message.Message{value},
	}
	if err := memory.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Validate error = %v", err)
	}
}

type failingSubmissionMessages struct {
	saved []message.Occurrence
}

func (messages *failingSubmissionMessages) CheckAppend(_ context.Context, requested []message.Message) error {
	for _, existing := range messages.saved {
		for _, value := range requested {
			if existing.Message.ID == value.ID {
				if existing.Message != value {
					return message.ErrIdentityConflict
				}
			}
		}
	}
	return nil
}

func (messages *failingSubmissionMessages) Append(
	_ context.Context,
	requested []message.Message,
) ([]message.Occurrence, error) {
	occurrences := make([]message.Occurrence, len(requested))
	for index, value := range requested {
		found := false
		for _, existing := range messages.saved {
			if existing.Message.ID == value.ID {
				if existing.Message != value {
					return nil, message.ErrIdentityConflict
				}
				occurrences[index] = existing
				found = true
				break
			}
		}
		if found {
			continue
		}
		occurrences[index] = message.Occurrence{
			Message:  value,
			Position: uint64(len(messages.saved)),
		}
		messages.saved = append(messages.saved, occurrences[index])
	}
	return occurrences, nil
}

type failingSubmission struct {
	nextErr     error
	submitCalls int
}

func (*failingSubmission) HasSource(context.Context, string) (bool, error) {
	return false, nil
}

func (*failingSubmission) Preflight(context.Context, submission.Command) error {
	return nil
}

func (failure *failingSubmission) Submit(context.Context, submission.Command) (submission.Result, error) {
	failure.submitCalls++
	err := failure.nextErr
	failure.nextErr = nil
	return submission.Result{}, err
}

type emptyConflictCatalog struct{}

func (emptyConflictCatalog) EntityConflict(
	context.Context,
	knowledge.EntityID,
) (resolution.EntityConflict, error) {
	return resolution.EntityConflict{}, nil
}

func (emptyConflictCatalog) RelationConflict(
	context.Context,
	knowledge.RelationID,
) (resolution.RelationConflict, error) {
	return resolution.RelationConflict{}, nil
}

func (emptyConflictCatalog) ClaimConflict(
	context.Context,
	knowledge.ClaimID,
) (resolution.ClaimConflict, error) {
	return resolution.ClaimConflict{}, nil
}
