package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/zone"
)

// EventAppender persists through the active transaction carried by ctx. Append
// must not begin, commit, or roll back a transaction of its own.
type EventAppender interface {
	Append(ctx context.Context, events []ProposedEvent) error
}

// Publisher validates registered events and appends them through the transaction
// carried by ctx.
type Publisher interface {
	Publish(ctx context.Context, events []ProposedEvent) error
}

// ServiceDependencies creates the Journal before domain services and consumer
// handlers are assembled.
type ServiceDependencies struct {
	Contracts     []EventContractRegistration
	AtomicBatches []AtomicBatchContract
	Reader        EventReader
}

// Service owns event registration, validation rules, and reads. Consumer
// scheduling and Publishers are assembled separately.
type Service struct {
	contracts     map[EventKey]EventContract
	atomicBatches []AtomicBatchContract
	reader        EventReader
}

// NewService freezes and validates the complete event contract set.
func NewService(dependencies ServiceDependencies) (*Service, error) {
	if dependencies.Reader == nil {
		return nil, errors.New("create Journal Service: Event Reader is required")
	}
	if len(dependencies.Contracts) == 0 {
		return nil, fmt.Errorf("%w: no Event Contracts were registered", ErrInvalidRegistration)
	}
	contracts := make(map[EventKey]EventContract, len(dependencies.Contracts))
	for index, registration := range dependencies.Contracts {
		if err := validateEventKey(registration.Key); err != nil {
			return nil, fmt.Errorf("%w: Event Contract %d: %v", ErrInvalidRegistration, index, err)
		}
		if registration.Contract == nil {
			return nil, fmt.Errorf("%w: Event Contract %s v%d is nil", ErrInvalidRegistration, registration.Key.Type, registration.Key.Version)
		}
		if _, exists := contracts[registration.Key]; exists {
			return nil, fmt.Errorf("%w: duplicate Event Contract %s v%d", ErrInvalidRegistration, registration.Key.Type, registration.Key.Version)
		}
		contracts[registration.Key] = registration.Contract
	}

	atomicBatches := make([]AtomicBatchContract, len(dependencies.AtomicBatches))
	completionOwners := make(map[EventKey]struct{}, len(dependencies.AtomicBatches))
	for index, batch := range dependencies.AtomicBatches {
		if len(batch.MemberEvents) == 0 {
			return nil, fmt.Errorf("%w: Atomic Batch %d has no member event types", ErrInvalidRegistration, index)
		}
		if _, exists := contracts[batch.CompletionEvent]; !exists {
			return nil, fmt.Errorf("%w: Atomic Batch %d completion %s v%d is not registered", ErrInvalidRegistration, index, batch.CompletionEvent.Type, batch.CompletionEvent.Version)
		}
		if _, exists := completionOwners[batch.CompletionEvent]; exists {
			return nil, fmt.Errorf("%w: completion %s v%d belongs to more than one Atomic Batch", ErrInvalidRegistration, batch.CompletionEvent.Type, batch.CompletionEvent.Version)
		}
		completionOwners[batch.CompletionEvent] = struct{}{}
		members := make(map[EventKey]struct{}, len(batch.MemberEvents))
		for _, member := range batch.MemberEvents {
			if member == batch.CompletionEvent {
				return nil, fmt.Errorf("%w: Atomic Batch %d completion is also a member", ErrInvalidRegistration, index)
			}
			if _, exists := contracts[member]; !exists {
				return nil, fmt.Errorf("%w: Atomic Batch %d member %s v%d is not registered", ErrInvalidRegistration, index, member.Type, member.Version)
			}
			if _, exists := members[member]; exists {
				return nil, fmt.Errorf("%w: Atomic Batch %d repeats member %s v%d", ErrInvalidRegistration, index, member.Type, member.Version)
			}
			members[member] = struct{}{}
		}
		atomicBatches[index] = AtomicBatchContract{
			MemberEvents:    append([]EventKey(nil), batch.MemberEvents...),
			CompletionEvent: batch.CompletionEvent,
		}
	}

	return &Service{
		contracts:     contracts,
		atomicBatches: atomicBatches,
		reader:        dependencies.Reader,
	}, nil
}

func (s *Service) supports(key EventKey) bool {
	if s == nil {
		return false
	}
	_, exists := s.contracts[key]
	return exists
}

func (s *Service) canonicalize(events []ProposedEvent) ([]ProposedEvent, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("%w: event batch is empty", ErrInvalidEvent)
	}
	result := make([]ProposedEvent, len(events))
	identities := make(map[EventID]struct{}, len(events))
	for index, event := range events {
		if _, duplicate := identities[event.EventID]; duplicate {
			return nil, fmt.Errorf("%w: event batch repeats EventID %q", ErrInvalidEvent, event.EventID)
		}
		identities[event.EventID] = struct{}{}
		key := EventKey{Type: event.Type, Version: event.SchemaVersion}
		contract, registered := s.contracts[key]
		if !registered {
			return nil, fmt.Errorf("%w: event %d uses unregistered contract %s v%d", ErrInvalidEvent, index, key.Type, key.Version)
		}
		canonical, err := contract.ValidateAndCanonicalize(event)
		if err != nil {
			return nil, fmt.Errorf("validate Journal event %d: %w", index, err)
		}
		result[index] = canonical
	}
	if err := s.validateAtomicBatches(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) validateAtomicBatches(events []ProposedEvent) error {
	for batchIndex, batch := range s.atomicBatches {
		members := make(map[EventKey]struct{}, len(batch.MemberEvents))
		for _, member := range batch.MemberEvents {
			members[member] = struct{}{}
		}
		involved := false
		completionIndex := -1
		completionCount := 0
		for index, event := range events {
			key := EventKey{Type: event.Type, Version: event.SchemaVersion}
			if _, member := members[key]; member {
				involved = true
			}
			if key == batch.CompletionEvent {
				involved = true
				completionIndex = index
				completionCount++
			}
		}
		if !involved {
			continue
		}
		if completionCount != 1 || completionIndex != len(events)-1 {
			return fmt.Errorf("%w: Atomic Batch %d requires one final %s v%d event", ErrInvalidEvent, batchIndex, batch.CompletionEvent.Type, batch.CompletionEvent.Version)
		}
		completionStream := events[completionIndex].StreamID
		for index, event := range events {
			key := EventKey{Type: event.Type, Version: event.SchemaVersion}
			_, member := members[key]
			if index != completionIndex && !member {
				return fmt.Errorf("%w: Atomic Batch %d contains unrelated %s v%d event", ErrInvalidEvent, batchIndex, key.Type, key.Version)
			}
			if event.StreamID != completionStream {
				return fmt.Errorf("%w: Atomic Batch %d spans more than one Stream", ErrInvalidEvent, batchIndex)
			}
		}
	}
	return nil
}

type publisher struct {
	journal  *Service
	appender EventAppender
}

var _ Publisher = (*publisher)(nil)

// NewPublisher binds Journal validation to its long-lived persistence port.
func NewPublisher(journal *Service, appender EventAppender) (Publisher, error) {
	if journal == nil {
		return nil, errors.New("create Journal Publisher: Service is required")
	}
	if appender == nil {
		return nil, errors.New("create Journal Publisher: Event Appender is required")
	}
	return &publisher{journal: journal, appender: appender}, nil
}

func (p *publisher) Publish(ctx context.Context, events []ProposedEvent) error {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return fmt.Errorf("publish Journal events: %w", err)
	}
	scoped := append([]ProposedEvent(nil), events...)
	for index := range scoped {
		if scoped[index].ZoneID != "" && scoped[index].ZoneID != zoneID {
			return fmt.Errorf(
				"%w: event %d belongs to Zone %q, Context belongs to Zone %q",
				ErrInvalidEvent,
				index,
				scoped[index].ZoneID,
				zoneID,
			)
		}
		scoped[index].ZoneID = zoneID
	}
	canonical, err := p.journal.canonicalize(scoped)
	if err != nil {
		return err
	}
	if err := p.appender.Append(ctx, canonical); err != nil {
		return fmt.Errorf("publish Journal events: %w", err)
	}
	return nil
}
