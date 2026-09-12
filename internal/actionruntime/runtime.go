// Package actionruntime runs one Zone-scoped controlled Action through an
// in-memory EventType boundary. Journal consumer positions are the only
// persisted delivery progress.
package actionruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/zone"
)

var (
	ErrEventIncomplete = errors.New("Action event remains incomplete")
	ErrNoPendingEvents = errors.New("Action has no pending events")
)

type PendingInput struct {
	Count uint64
	Since time.Time
}

type Dependencies struct {
	Name       string
	ConsumerID journal.ConsumerID
	Journal    *journal.Service
	Positions  journal.ConsumerPositions
	EventKeys  []journal.EventKey
	Handle     func(context.Context, journal.Event) error
	ReadLimit  int
}

type Runtime struct {
	name       string
	consumerID journal.ConsumerID
	journal    *journal.Service
	positions  journal.ConsumerPositions
	eventKeys  []journal.EventKey
	handle     func(context.Context, journal.Event) error
	readLimit  int
	wake       chan struct{}

	mutex    sync.Mutex
	requests map[zone.ID]map[journal.EventType]journal.Position
	active   map[zone.ID]map[journal.EventType]journal.Position
	failed   map[zone.ID]map[journal.EventType]journal.Position
}

func New(dependencies Dependencies) (*Runtime, error) {
	switch {
	case dependencies.Name == "":
		return nil, errors.New("create Action runtime: Name is required")
	case dependencies.ConsumerID == "":
		return nil, fmt.Errorf("create %s runtime: ConsumerID is required", dependencies.Name)
	case dependencies.Journal == nil:
		return nil, fmt.Errorf("create %s runtime: Journal is required", dependencies.Name)
	case dependencies.Positions == nil:
		return nil, fmt.Errorf("create %s runtime: Consumer Positions are required", dependencies.Name)
	case len(dependencies.EventKeys) == 0:
		return nil, fmt.Errorf("create %s runtime: EventKeys are required", dependencies.Name)
	case dependencies.Handle == nil:
		return nil, fmt.Errorf("create %s runtime: Handle is required", dependencies.Name)
	case dependencies.ReadLimit <= 0:
		return nil, fmt.Errorf("create %s runtime: ReadLimit must be positive", dependencies.Name)
	}
	eventTypes := make(map[journal.EventType]struct{}, len(dependencies.EventKeys))
	for _, key := range dependencies.EventKeys {
		if _, duplicate := eventTypes[key.Type]; duplicate {
			return nil, fmt.Errorf("create %s runtime: EventType %q is repeated", dependencies.Name, key.Type)
		}
		eventTypes[key.Type] = struct{}{}
	}
	return &Runtime{
		name:       dependencies.Name,
		consumerID: dependencies.ConsumerID,
		journal:    dependencies.Journal,
		positions:  dependencies.Positions,
		eventKeys:  append([]journal.EventKey(nil), dependencies.EventKeys...),
		handle:     dependencies.Handle,
		readLimit:  dependencies.ReadLimit,
		wake:       make(chan struct{}, 1),
		requests:   make(map[zone.ID]map[journal.EventType]journal.Position),
		active:     make(map[zone.ID]map[journal.EventType]journal.Position),
		failed:     make(map[zone.ID]map[journal.EventType]journal.Position),
	}, nil
}

func (runtime *Runtime) Pending(ctx context.Context) (PendingInput, error) {
	if runtime == nil {
		return PendingInput{}, errors.New("read pending Action input: runtime is required")
	}
	if _, err := zone.RequireID(ctx); err != nil {
		return PendingInput{}, err
	}
	result := PendingInput{}
	for _, key := range runtime.eventKeys {
		position, err := runtime.positions.Position(ctx, runtime.consumerID, key.Type)
		if err != nil {
			return PendingInput{}, fmt.Errorf("read %s %q Position: %w", runtime.name, key.Type, err)
		}
		pending, err := runtime.journal.PendingEvents(ctx, key, position)
		if err != nil {
			return PendingInput{}, err
		}
		result.Count += pending.Count
		if result.Since.IsZero() || (!pending.Since.IsZero() && pending.Since.Before(result.Since)) {
			result.Since = pending.Since
		}
	}
	return result, nil
}

// Start schedules pending events unless the same EventType boundary already
// failed in this process. A new boundary becomes eligible automatically.
func (runtime *Runtime) Start(ctx context.Context) error {
	return runtime.request(ctx, false)
}

// Retry explicitly schedules the current Zone even when its current boundary
// already failed in this process.
func (runtime *Runtime) Retry(ctx context.Context) error {
	return runtime.request(ctx, true)
}

func (runtime *Runtime) request(ctx context.Context, retry bool) error {
	if runtime == nil {
		return errors.New("start Action: runtime is required")
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	boundaries := make(map[journal.EventType]journal.Position, len(runtime.eventKeys))
	for _, key := range runtime.eventKeys {
		position, err := runtime.positions.Position(ctx, runtime.consumerID, key.Type)
		if err != nil {
			return err
		}
		pending, err := runtime.journal.PendingEvents(ctx, key, position)
		if err != nil {
			return err
		}
		if pending.Count > 0 {
			boundaries[key.Type] = pending.ThroughPosition
		}
	}
	if len(boundaries) == 0 {
		if !retry {
			return nil
		}
		return ErrNoPendingEvents
	}

	runtime.mutex.Lock()
	for eventType, through := range boundaries {
		if runtime.active[zoneID][eventType] >= through {
			delete(boundaries, eventType)
		}
	}
	if !retry {
		for eventType, through := range boundaries {
			if runtime.failed[zoneID][eventType] >= through {
				delete(boundaries, eventType)
			}
		}
	}
	if len(boundaries) == 0 {
		runtime.mutex.Unlock()
		return nil
	}
	if retry {
		delete(runtime.failed, zoneID)
	}
	requested := runtime.requests[zoneID]
	if requested == nil {
		requested = make(map[journal.EventType]journal.Position, len(boundaries))
		runtime.requests[zoneID] = requested
	}
	for eventType, through := range boundaries {
		if through > requested[eventType] {
			requested[eventType] = through
		}
	}
	runtime.mutex.Unlock()

	select {
	case runtime.wake <- struct{}{}:
	default:
	}
	return nil
}

func (runtime *Runtime) Run(ctx context.Context) error {
	if runtime == nil {
		return errors.New("run Action: runtime is required")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-runtime.wake:
			for {
				zoneID, boundaries, found := runtime.takeRequest()
				if !found {
					break
				}
				if err := runtime.processZone(ctx, zoneID, boundaries); err != nil {
					return err
				}
				runtime.finishRequest(zoneID)
			}
		}
	}
}

func (runtime *Runtime) takeRequest() (
	zone.ID,
	map[journal.EventType]journal.Position,
	bool,
) {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	for zoneID, boundaries := range runtime.requests {
		delete(runtime.requests, zoneID)
		runtime.active[zoneID] = boundaries
		return zoneID, boundaries, true
	}
	return "", nil, false
}

func (runtime *Runtime) finishRequest(zoneID zone.ID) {
	runtime.mutex.Lock()
	delete(runtime.active, zoneID)
	runtime.mutex.Unlock()
}

func (runtime *Runtime) processZone(
	ctx context.Context,
	zoneID zone.ID,
	boundaries map[journal.EventType]journal.Position,
) error {
	zoneContext, err := zone.NewContext(ctx, zoneID)
	if err != nil {
		return err
	}
	for _, key := range runtime.eventKeys {
		through, requested := boundaries[key.Type]
		if !requested {
			continue
		}
		if err := runtime.processEventType(zoneContext, key, through); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *Runtime) processEventType(
	ctx context.Context,
	key journal.EventKey,
	through journal.Position,
) error {
	position, err := runtime.positions.Position(ctx, runtime.consumerID, key.Type)
	if err != nil {
		return err
	}
	for position < through {
		events, err := runtime.journal.Events(ctx, key.Type, position, through, runtime.readLimit)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("process %s: EventType %q ended before Position %d", runtime.name, key.Type, through)
		}
		for _, event := range events {
			if event.Key() == key {
				if err := runtime.handle(ctx, event); err != nil {
					if errors.Is(err, ErrEventIncomplete) {
						runtime.markFailed(event.ZoneID, key.Type, through)
						return nil
					}
					return err
				}
			}
			next := journal.Position(event.Sequence) + 1
			if err := runtime.positions.Advance(
				ctx,
				runtime.consumerID,
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

func (runtime *Runtime) markFailed(
	zoneID zone.ID,
	eventType journal.EventType,
	through journal.Position,
) {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	failed := runtime.failed[zoneID]
	if failed == nil {
		failed = make(map[journal.EventType]journal.Position)
		runtime.failed[zoneID] = failed
	}
	failed[eventType] = through
}
