package assembly

import (
	"context"
	"time"

	"github.com/memoria-space/meking/mcp"
	"github.com/memoria-space/meking/memory/activation"
	activationcorpus "github.com/memoria-space/meking/memory/activation/adapter/corpus"
	activationknowledge "github.com/memoria-space/meking/memory/activation/adapter/knowledge"
	activationsqlite "github.com/memoria-space/meking/memory/activation/adapter/sqlite"
)

func (service *Service) openActivation(ctx context.Context) error {
	store, err := activationsqlite.New(service.resources.database)
	if err != nil {
		return err
	}
	model, err := activation.NewModel(service.configuration.Activation.Model)
	if err != nil {
		return err
	}
	protocols, err := mcp.RecallProtocols(service.configuration.Activation.ChinesePrompt, service.configuration.Activation.EnglishPrompt)
	if err != nil {
		return err
	}
	if err := activation.Prepare(ctx, service.resources.transactions, store, model, protocols); err != nil {
		return err
	}
	targets, err := activationknowledge.New(service.versions)
	if err != nil {
		return err
	}
	evidence, err := activationcorpus.New(service.messages)
	if err != nil {
		return err
	}
	service.observations, err = activation.NewService(activation.Dependencies{Tx: service.resources.transactions, Store: store, Targets: targets, Evidence: evidence, Protocols: protocols, Model: model, Now: time.Now})
	return err
}

func (service *Service) Observations() *activation.Service {
	if service == nil {
		return nil
	}
	return service.observations
}
