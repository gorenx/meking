package assembly

import (
	"errors"
	"fmt"
	"time"

	journalcore "github.com/memoria-space/meking/journal"
)

const (
	dispatchReadLimit    = 128
	dispatchPollInterval = 100 * time.Millisecond
)

func newDispatcher(
	events *journalcore.Service,
	zones journalcore.ZoneResolver,
	positions journalcore.ConsumerPositions,
	consumers []journalcore.Consumer,
) (*journalcore.Dispatcher, error) {
	if events == nil || zones == nil || positions == nil {
		return nil, errors.New("assemble Journal Dispatcher: Journal, Zones, and positions are required")
	}
	dispatcher, err := journalcore.NewDispatcher(journalcore.DispatcherDependencies{
		Journal:      events,
		Zones:        zones,
		Consumers:    append([]journalcore.Consumer(nil), consumers...),
		Positions:    positions,
		ReadLimit:    dispatchReadLimit,
		PollInterval: dispatchPollInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble Journal Dispatcher: %w", err)
	}
	return dispatcher, nil
}
