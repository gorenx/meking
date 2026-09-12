package application

import (
	"context"
	"testing"
	"time"

	"github.com/memoria-space/meking/controlplane"
	"github.com/memoria-space/meking/zone"
)

type automaticZoneCatalog struct {
	definitions []zone.Definition
}

func (catalog automaticZoneCatalog) List(context.Context) ([]zone.Definition, error) {
	return catalog.definitions, nil
}

type automaticActionRecorder struct {
	invocations map[zone.ID]int
}

func (recorder *automaticActionRecorder) InvokeAutomatic(
	ctx context.Context,
	_ controlplane.Action,
	_ time.Time,
) (Invocation, bool, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return Invocation{}, false, err
	}
	recorder.invocations[zoneID]++
	return Invocation{}, false, nil
}

func TestAutomaticEvaluatorInvokesEachZoneSeparately(t *testing.T) {
	first, err := zone.NewRootDefinition(
		"10000000-0000-4000-8000-000000000001",
		"",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := zone.NewRootDefinition(
		"20000000-0000-4000-8000-000000000002",
		"",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &automaticActionRecorder{
		invocations: make(map[zone.ID]int),
	}
	evaluator := &AutomaticEvaluator{
		zones: automaticZoneCatalog{
			definitions: []zone.Definition{first, second},
		},
		actions: recorder,
	}
	if err := evaluator.scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, definition := range []zone.Definition{first, second} {
		if recorder.invocations[definition.ID] != len(controlplane.Actions()) {
			t.Fatalf(
				"Zone %q invocation count = %d, want %d",
				definition.ID,
				recorder.invocations[definition.ID],
				len(controlplane.Actions()),
			)
		}
	}
}
