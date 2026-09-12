package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/community/structurebuild"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
	knowledgeevents "github.com/memoria-space/meking/knowledge/integration"
)

type StructureBuilder interface {
	Build(context.Context, structurebuild.Input) (community.Structure, error)
}

type StructureDerivationActionDependencies struct {
	Journal      *journalcore.Service
	Positions    journalcore.ConsumerPositions
	Builder      StructureBuilder
	ReadLimit    int
}

type StructureDerivationAction struct {
	runtime *actionruntime.Runtime
	builder StructureBuilder
}

func NewStructureDerivationAction(
	dependencies StructureDerivationActionDependencies,
) (*StructureDerivationAction, error) {
	if dependencies.Builder == nil {
		return nil, errors.New("create Community Structure Derivation Action: Builder is required")
	}
	action := &StructureDerivationAction{builder: dependencies.Builder}
	runtime, err := newActionRuntime(
		"Community Structure Derivation",
		"community.structure-derivation",
		actionRuntimeDependencies{
			Journal:   dependencies.Journal,
			Positions: dependencies.Positions,
			ReadLimit: dependencies.ReadLimit,
		},
		journalcore.EventKeyFor[knowledgeevents.PublishedV1](),
		action.buildStructure,
	)
	if err != nil {
		return nil, err
	}
	action.runtime = runtime
	return action, nil
}

func (action *StructureDerivationAction) Pending(ctx context.Context) (actionruntime.PendingInput, error) {
	if action == nil {
		return actionruntime.PendingInput{}, errors.New("read pending Community Structure input: Action is required")
	}
	return action.runtime.Pending(ctx)
}

func (action *StructureDerivationAction) Start(ctx context.Context) error {
	if action == nil {
		return errors.New("start Community Structure Derivation: Action is required")
	}
	return action.runtime.Start(ctx)
}

func (action *StructureDerivationAction) Retry(ctx context.Context) error {
	if action == nil {
		return errors.New("retry Community Structure Derivation: Action is required")
	}
	return action.runtime.Retry(ctx)
}

func (action *StructureDerivationAction) Run(ctx context.Context) error {
	if action == nil {
		return errors.New("run Community Structure Derivation Action: Action is required")
	}
	return action.runtime.Run(ctx)
}

func (action *StructureDerivationAction) buildStructure(
	ctx context.Context,
	event journalcore.Event,
) error {
	body, err := journalcore.DecodeJSONBody[knowledgeevents.PublishedV1](event)
	if err != nil {
		return fmt.Errorf("decode published Knowledge for Community Structure: %w", err)
	}
	_, err = action.builder.Build(ctx, structurebuild.Input{
		SourceEventID: string(event.EventID),
		CorrelationID: string(event.CorrelationID),
		CorporaID:     string(body.CorporaID),
		Knowledge:     body.Knowledge,
	})
	if err != nil {
		return fmt.Errorf("build Community Structure from Knowledge publication %q: %w", event.EventID, err)
	}
	return nil
}
