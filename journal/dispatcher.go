package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/zone"
	"golang.org/x/sync/errgroup"
)

type ZoneResolver interface {
	Resolve(ctx context.Context, id zone.ID) (zone.Definition, error)
}

type DispatcherDependencies struct {
	Journal      *Service
	Zones        ZoneResolver
	Consumers    []Consumer
	Positions    ConsumerPositions
	ReadLimit    int
	PollInterval time.Duration
}

type Dispatcher struct {
	journal   *Service
	zones     ZoneResolver
	consumers []registeredConsumer
	positions ConsumerPositions
	readLimit int
	pollDelay time.Duration
}

type registeredConsumer struct {
	handler Consumer
	id      ConsumerID
	keys    []EventKey
	events  map[EventKey]struct{}
}

func NewDispatcher(dependencies DispatcherDependencies) (*Dispatcher, error) {
	switch {
	case dependencies.Journal == nil:
		return nil, errors.New("create Journal Dispatcher: Service is required")
	case dependencies.Zones == nil:
		return nil, errors.New("create Journal Dispatcher: Zone Resolver is required")
	case dependencies.Positions == nil:
		return nil, errors.New("create Journal Dispatcher: Consumer Positions are required")
	case len(dependencies.Consumers) == 0:
		return nil, fmt.Errorf("%w: no Consumers were registered", ErrInvalidRegistration)
	case dependencies.ReadLimit <= 0:
		return nil, errors.New("create Journal Dispatcher: Read Limit must be positive")
	case dependencies.PollInterval <= 0:
		return nil, errors.New("create Journal Dispatcher: Poll Interval must be positive")
	}
	consumers := make([]registeredConsumer, len(dependencies.Consumers))
	ids := make(map[ConsumerID]struct{}, len(dependencies.Consumers))
	for index, consumer := range dependencies.Consumers {
		if consumer == nil {
			return nil, fmt.Errorf("%w: Consumer %d is nil", ErrInvalidRegistration, index)
		}
		registered, err := registerConsumer(dependencies.Journal, consumer)
		if err != nil {
			return nil, fmt.Errorf("%w: Consumer %d: %v", ErrInvalidRegistration, index, err)
		}
		if _, duplicate := ids[registered.id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate ConsumerID %q", ErrInvalidRegistration, registered.id)
		}
		ids[registered.id] = struct{}{}
		consumers[index] = registered
	}
	return &Dispatcher{
		journal:   dependencies.Journal,
		zones:     dependencies.Zones,
		consumers: consumers,
		positions: dependencies.Positions,
		readLimit: dependencies.ReadLimit,
		pollDelay: dependencies.PollInterval,
	}, nil
}

func registerConsumer(service *Service, consumer Consumer) (registeredConsumer, error) {
	registration := consumer.ConsumerRegistration()
	events, err := validateConsumerRegistration(service, registration)
	if err != nil {
		return registeredConsumer{}, err
	}
	return registeredConsumer{
		handler: consumer,
		id:      registration.ID,
		keys:    append([]EventKey(nil), registration.Events...),
		events:  events,
	}, nil
}

func validateConsumerRegistration(
	service *Service,
	registration ConsumerRegistration,
) (map[EventKey]struct{}, error) {
	if service == nil {
		return nil, errors.New("Journal Service is required")
	}
	if err := validateIdentity(string(registration.ID), "ConsumerID"); err != nil {
		return nil, err
	}
	if len(registration.Events) == 0 {
		return nil, fmt.Errorf("Consumer %q handles no events", registration.ID)
	}
	events := make(map[EventKey]struct{}, len(registration.Events))
	eventTypes := make(map[EventType]struct{}, len(registration.Events))
	for _, key := range registration.Events {
		if !service.supports(key) {
			return nil, fmt.Errorf(
				"Consumer %q handles unregistered event %s v%d",
				registration.ID,
				key.Type,
				key.Version,
			)
		}
		if _, duplicate := events[key]; duplicate {
			return nil, fmt.Errorf("Consumer %q repeats event %s v%d", registration.ID, key.Type, key.Version)
		}
		if _, duplicate := eventTypes[key.Type]; duplicate {
			return nil, fmt.Errorf("Consumer %q repeats EventType %s", registration.ID, key.Type)
		}
		eventTypes[key.Type] = struct{}{}
		events[key] = struct{}{}
	}
	return events, nil
}

func (dispatcher *Dispatcher) Run(ctx context.Context) error {
	group, runContext := errgroup.WithContext(ctx)
	for _, consumer := range dispatcher.consumers {
		consumer := consumer
		group.Go(func() error {
			if err := dispatcher.runConsumer(runContext, consumer); err != nil {
				return fmt.Errorf("run Journal Consumer %q: %w", consumer.id, err)
			}
			return nil
		})
	}
	return group.Wait()
}

func (dispatcher *Dispatcher) runConsumer(
	ctx context.Context,
	consumer registeredConsumer,
) error {
	for {
		processed := false
		for _, key := range consumer.keys {
			var err error
			processed, err = dispatcher.processPending(ctx, consumer, key)
			if err != nil {
				return err
			}
			if processed {
				break
			}
		}
		if processed {
			continue
		}
		timer := time.NewTimer(dispatcher.pollDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (dispatcher *Dispatcher) processPending(
	ctx context.Context,
	consumer registeredConsumer,
	key EventKey,
) (bool, error) {
	zoneID, err := dispatcher.positions.PendingZone(ctx, consumer.id, key)
	if err != nil {
		return false, fmt.Errorf("read pending Zone for Consumer %q: %w", consumer.id, err)
	}
	if zoneID == "" {
		return false, nil
	}
	definition, err := dispatcher.zones.Resolve(ctx, zoneID)
	if err != nil {
		return false, fmt.Errorf("resolve pending Zone %q: %w", zoneID, err)
	}
	zoneContext, err := zone.NewContext(ctx, definition.ID)
	if err != nil {
		return false, err
	}
	if err := dispatcher.processEventType(zoneContext, consumer, key); err != nil {
		return false, err
	}
	return true, nil
}

func (dispatcher *Dispatcher) processEventType(
	ctx context.Context,
	consumer registeredConsumer,
	key EventKey,
) error {
	position, err := dispatcher.positions.Position(ctx, consumer.id, key.Type)
	if err != nil {
		return fmt.Errorf("load Consumer %q EventType %q Position: %w", consumer.id, key.Type, err)
	}
	through, err := dispatcher.journal.EventTypePosition(ctx, key.Type)
	if err != nil {
		return err
	}
	for position < through {
		events, err := dispatcher.journal.Events(ctx, key.Type, position, through, dispatcher.readLimit)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("EventType %q ended before Position %d", key.Type, through)
		}
		for _, event := range events {
			if event.Key() == key {
				if err := dispatcher.handleCausalStream(ctx, consumer, event); err != nil {
					return fmt.Errorf(
						"handle EventID %q at %s Sequence %d: %w",
						event.EventID,
						event.Type,
						event.Sequence,
						err,
					)
				}
			}
			next := Position(event.Sequence) + 1
			if err := dispatcher.positions.Advance(
				ctx,
				consumer.id,
				key.Type,
				position,
				next,
			); err != nil {
				return err
			}
			position = next
		}
	}
	return nil
}

// handleCausalStream follows only the direct same-Stream continuation emitted
// by the event just handled. Followed events are delivered again by their own
// EventType consumer and therefore remain idempotent.
func (dispatcher *Dispatcher) handleCausalStream(
	ctx context.Context,
	consumer registeredConsumer,
	event Event,
) error {
	current := event
	for {
		if err := consumer.handler.Handle(ctx, current); err != nil {
			return err
		}
		next, err := dispatcher.journal.StreamEventsAfter(
			ctx,
			current.StreamID,
			current.StreamSequence,
			1,
		)
		if err != nil {
			return err
		}
		if len(next) == 0 || next[0].CausationID != current.EventID {
			return nil
		}
		if _, handles := consumer.events[next[0].Key()]; !handles {
			return nil
		}
		current = next[0]
	}
}
