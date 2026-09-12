package assembly

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/memoria-space/meking/journal"
	"golang.org/x/sync/errgroup"
)

type processRunner interface {
	Run(context.Context) error
}

type processRuntimeDependencies struct {
	Journal           *journal.Service
	Zones             journal.ZoneResolver
	ConsumerPositions journal.ConsumerPositions
	Consumers         []journal.Consumer
	Runners           []processRunner
}

type processRuntime struct {
	dispatcher *journal.Dispatcher
	runners    []processRunner

	mutex   sync.RWMutex
	running bool
	closed  bool
}

func newProcessRuntime(dependencies processRuntimeDependencies) (*processRuntime, error) {
	switch {
	case dependencies.Journal == nil:
		return nil, errors.New("create process runtime: Journal is required")
	case dependencies.ConsumerPositions == nil:
		return nil, errors.New("create process runtime: Consumer positions are required")
	case dependencies.Zones == nil:
		return nil, errors.New("create process runtime: Zone Resolver is required")
	case len(dependencies.Consumers) == 0:
		return nil, errors.New("create process runtime: Consumers are required")
	case len(dependencies.Runners) == 0:
		return nil, errors.New("create process runtime: Action Runners are required")
	}
	dispatcher, err := newDispatcher(
		dependencies.Journal,
		dependencies.Zones,
		dependencies.ConsumerPositions,
		dependencies.Consumers,
	)
	if err != nil {
		return nil, err
	}
	runners := append([]processRunner(nil), dependencies.Runners...)
	for index, runner := range runners {
		if runner == nil {
			return nil, fmt.Errorf("create process runtime: Runner %d is required", index)
		}
	}
	return &processRuntime{
		dispatcher: dispatcher,
		runners:    runners,
	}, nil
}

// Run dispatches committed events from persisted Consumer positions.
func (runtime *processRuntime) Run(ctx context.Context) error {
	if runtime == nil {
		return errors.New("run process runtime: runtime is required")
	}
	runtime.mutex.Lock()
	if runtime.closed {
		runtime.mutex.Unlock()
		return errors.New("run process runtime: runtime is closed")
	}
	if runtime.running {
		runtime.mutex.Unlock()
		return errors.New("run process runtime: runtime is already running")
	}
	runtime.running = true
	runtime.mutex.Unlock()
	defer func() {
		runtime.mutex.Lock()
		runtime.running = false
		runtime.mutex.Unlock()
	}()

	group, runContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		return runtime.dispatcher.Run(runContext)
	})
	for _, runner := range runtime.runners {
		runner := runner
		group.Go(func() error {
			return runner.Run(runContext)
		})
	}
	dispatchErr := group.Wait()
	callerErr := ctx.Err()
	if dispatchErr != nil && (callerErr == nil || !errors.Is(dispatchErr, callerErr)) {
		return fmt.Errorf("run process runtime: %w", dispatchErr)
	}
	return nil
}

func (runtime *processRuntime) Close() error {
	if runtime == nil {
		return nil
	}
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	if runtime.closed {
		return nil
	}
	if runtime.running {
		return errors.New("close process runtime: runtime is still running")
	}
	runtime.closed = true
	runtime.dispatcher = nil
	runtime.runners = nil
	return nil
}
