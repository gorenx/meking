package assembly

import (
	"context"
	"errors"
	"time"

	"github.com/memoria-space/meking/controlplane"
	controlactions "github.com/memoria-space/meking/controlplane/adapter/actions"
	controlsqlite "github.com/memoria-space/meking/controlplane/adapter/sqlite"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
	"github.com/memoria-space/meking/internal/actionruntime"
	"github.com/memoria-space/meking/zone"
)

type controlPlaneApplications struct {
	policies *controlapplication.Policies
}

type actionApplications struct {
	invocations *controlapplication.Actions
	statuses    *controlapplication.ActionStatuses
}

type controlledAction interface {
	Pending(context.Context) (actionruntime.PendingInput, error)
	Start(context.Context) error
	Retry(context.Context) error
}

func routeAction(action controlledAction) controlactions.Consumer {
	return controlactions.Consumer{
		Pending: func(ctx context.Context) (controlapplication.PendingInput, error) {
			pending, err := action.Pending(ctx)
			return controlapplication.PendingInput{
				Count: pending.Count,
				Since: pending.Since,
			}, err
		},
		Start: action.Start,
		Retry: func(ctx context.Context) error {
			err := action.Retry(ctx)
			if errors.Is(err, actionruntime.ErrNoPendingEvents) {
				return controlplane.ErrNoPendingInput
			}
			return err
		},
	}
}

func openAutomaticControl(
	zones *zone.Catalog,
	actions *controlapplication.Actions,
	wakeups controlapplication.AutomaticWakeups,
) (*controlapplication.AutomaticEvaluator, error) {
	const recoveryInterval = 100 * time.Millisecond
	evaluator, err := controlapplication.NewAutomaticEvaluator(
		zones,
		actions,
		wakeups,
		recoveryInterval,
	)
	if err != nil {
		return nil, err
	}
	return evaluator, nil
}

func openControlActions(
	policies *controlapplication.Policies,
	corpora corpusApplications,
	knowledge knowledgeApplications,
	communities communityApplications,
	epochs epochApplications,
) (actionApplications, error) {
	consumer, err := controlactions.NewRouter(map[controlplane.Action]controlactions.Consumer{
		controlplane.ConvertDocument:          routeAction(corpora.documentConversion),
		controlplane.CreateTextUnits:          routeAction(corpora.textUnitCreation),
		controlplane.ExtractKnowledge:         routeAction(knowledge.extractionAction),
		controlplane.IndexEntityVectors:       routeAction(knowledge.entityVectorAction),
		controlplane.DeriveCommunityStructure: routeAction(communities.structureDerivation),
		controlplane.PublishEpoch:             routeAction(epochs.publicationAction),
		controlplane.GenerateCommunityReports: routeAction(communities.reportGeneration),
	})
	if err != nil {
		return actionApplications{}, err
	}
	invocations, err := controlapplication.NewActions(controlapplication.ActionDependencies{
		Policies: policies,
		Consumer: consumer,
	})
	if err != nil {
		return actionApplications{}, err
	}
	statuses, err := controlapplication.NewActionStatuses(policies, consumer)
	if err != nil {
		return actionApplications{}, err
	}
	return actionApplications{
		invocations: invocations,
		statuses:    statuses,
	}, nil
}

func openControlPlane(resources databaseResources) (controlPlaneApplications, error) {
	store, err := controlsqlite.NewStore(resources.database)
	if err != nil {
		return controlPlaneApplications{}, err
	}
	policies, err := controlapplication.NewPolicies(resources.transactions, store)
	if err != nil {
		return controlPlaneApplications{}, err
	}
	return controlPlaneApplications{
		policies: policies,
	}, nil
}
