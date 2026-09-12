package journal

import (
	"context"

	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
)

type actionRuntimeDependencies struct {
	Journal   *journalcore.Service
	Positions journalcore.ConsumerPositions
	ReadLimit int
}

func newActionRuntime(
	name string,
	consumerID journalcore.ConsumerID,
	dependencies actionRuntimeDependencies,
	eventKey journalcore.EventKey,
	handle func(context.Context, journalcore.Event) error,
) (*actionruntime.Runtime, error) {
	return actionruntime.New(actionruntime.Dependencies{
		Name:       name,
		ConsumerID: consumerID,
		Journal:    dependencies.Journal,
		Positions:  dependencies.Positions,
		EventKeys:  []journalcore.EventKey{eventKey},
		Handle:     handle,
		ReadLimit:  dependencies.ReadLimit,
	})
}
