package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/community/reportgeneration"
	epochevents "github.com/memoria-space/meking/epoch/integration"
	"github.com/memoria-space/meking/internal/actionruntime"
	journalcore "github.com/memoria-space/meking/journal"
)

type ReportBuilder interface {
	Build(context.Context, reportgeneration.Input) (communityreport.Publication, error)
}

type ReportGenerationActionDependencies struct {
	Journal      *journalcore.Service
	Positions    journalcore.ConsumerPositions
	Builder      ReportBuilder
	ReadLimit    int
}

type ReportGenerationAction struct {
	runtime *actionruntime.Runtime
	builder ReportBuilder
}

func NewReportGenerationAction(
	dependencies ReportGenerationActionDependencies,
) (*ReportGenerationAction, error) {
	if dependencies.Builder == nil {
		return nil, errors.New("create Community Report Generation Action: Builder is required")
	}
	action := &ReportGenerationAction{builder: dependencies.Builder}
	runtime, err := newActionRuntime(
		"Community Report Generation",
		"community.report-generation",
		actionRuntimeDependencies{
			Journal:   dependencies.Journal,
			Positions: dependencies.Positions,
			ReadLimit: dependencies.ReadLimit,
		},
		journalcore.EventKeyFor[epochevents.PublishedV1](),
		action.buildReports,
	)
	if err != nil {
		return nil, err
	}
	action.runtime = runtime
	return action, nil
}

func (action *ReportGenerationAction) Pending(ctx context.Context) (actionruntime.PendingInput, error) {
	if action == nil {
		return actionruntime.PendingInput{}, errors.New("read pending Community Report input: Action is required")
	}
	return action.runtime.Pending(ctx)
}

func (action *ReportGenerationAction) Start(ctx context.Context) error {
	if action == nil {
		return errors.New("start Community Report Generation: Action is required")
	}
	return action.runtime.Start(ctx)
}

func (action *ReportGenerationAction) Retry(ctx context.Context) error {
	if action == nil {
		return errors.New("retry Community Report Generation: Action is required")
	}
	return action.runtime.Retry(ctx)
}

func (action *ReportGenerationAction) Run(ctx context.Context) error {
	if action == nil {
		return errors.New("run Community Report Generation Action: Action is required")
	}
	return action.runtime.Run(ctx)
}

func (action *ReportGenerationAction) buildReports(
	ctx context.Context,
	event journalcore.Event,
) error {
	if err := epochevents.ValidateStream(string(event.StreamID)); err != nil {
		return err
	}
	body, err := journalcore.DecodeJSONBody[epochevents.PublishedV1](event)
	if err != nil {
		return fmt.Errorf("decode Epoch for Community Report Generation: %w", err)
	}
	_, err = action.builder.Build(ctx, reportgeneration.Input{
		EpochID:          int64(body.EpochID),
		StructureID:      community.StructureID(body.StructureID),
		CorporaID:        string(body.CorporaID),
		Knowledge:        body.Knowledge,
		EpochPublishedAt: event.OccurredAt,
	})
	if err != nil {
		return fmt.Errorf("generate Reports for Epoch %d: %w", body.EpochID, err)
	}
	return nil
}
