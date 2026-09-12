package extraction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/journal"
	"github.com/memoria-space/meking/knowledge"
	knowledgeevents "github.com/memoria-space/meking/knowledge/integration"
	"github.com/memoria-space/meking/knowledge/submission"
)

func TestApplicationResumesAfterLastSubmittedTextUnit(t *testing.T) {
	dependencies := &applicationDependencies{
		units:       []TextUnitInput{{ID: "one", Text: "first"}, {ID: "two", Text: "second"}, {ID: "three", Text: "third"}},
		last:        "one",
		hasProgress: true,
		versions:    knowledge.Manifest{Entities: []knowledge.Reference[knowledge.EntityID]{{ID: "10000000-0000-4000-8000-000000000001", Version: 1}}},
	}
	application, err := NewApplication(Dependencies{
		Tx: dependencies, TextUnitReader: dependencies, Progress: dependencies,
		KnowledgeSubmissions: dependencies, Manifests: dependencies,
		Producer: dependencies, Extractor: dependencies,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := application.ExtractCorpora(context.Background(), Request{
		CorporaID: "corpora", SourceEventID: "event/source", CorrelationID: "correlation",
		RequestedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.extracted) != 2 || dependencies.extracted[0] != "two" || dependencies.extracted[1] != "three" {
		t.Fatalf("extracted TextUnits = %v", dependencies.extracted)
	}
	if len(dependencies.submitted) != 2 || dependencies.advances != 2 || dependencies.last != "three" {
		t.Fatalf("submitted=%d advances=%d last=%q", len(dependencies.submitted), dependencies.advances, dependencies.last)
	}
	if dependencies.published != 1 || got.Entities[0].ID != "10000000-0000-4000-8000-000000000001" {
		t.Fatalf("published=%d versions=%#v", dependencies.published, got)
	}
}

func TestApplicationDoesNotSubmitOrAdvanceRejectedExtraction(t *testing.T) {
	rejected := errors.New("invalid corrected extraction")
	dependencies := &applicationDependencies{
		units:      []TextUnitInput{{ID: "one", Text: "first"}},
		extractErr: rejected,
	}
	application, err := NewApplication(Dependencies{
		Tx: dependencies, TextUnitReader: dependencies, Progress: dependencies,
		KnowledgeSubmissions: dependencies, Manifests: dependencies,
		Producer: dependencies, Extractor: dependencies,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.ExtractCorpora(t.Context(), Request{
		CorporaID: "corpora", SourceEventID: "event/source", CorrelationID: "correlation",
		RequestedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, rejected) {
		t.Fatalf("ExtractCorpora() error = %v", err)
	}
	if len(dependencies.submitted) != 0 || dependencies.advances != 0 || dependencies.published != 0 {
		t.Fatalf(
			"submitted/advances/published = %d/%d/%d",
			len(dependencies.submitted), dependencies.advances, dependencies.published,
		)
	}
}

type applicationDependencies struct {
	units       []TextUnitInput
	last        string
	hasProgress bool
	versions    knowledge.Manifest
	extracted   []string
	submitted   []submission.Command
	advances    int
	published   int
	extractErr  error
}

func (d *applicationDependencies) WithTx(ctx context.Context, work func(context.Context) error) error {
	return work(ctx)
}
func (d *applicationDependencies) TextUnits(context.Context, string) ([]TextUnitInput, error) {
	return append([]TextUnitInput(nil), d.units...), nil
}
func (d *applicationDependencies) ExtractionProgress(context.Context, string) (string, bool, error) {
	return d.last, d.hasProgress, nil
}
func (d *applicationDependencies) AdvanceExtraction(_ context.Context, _ string, expected string, last string) error {
	if expected != d.last {
		return ErrExtractionProgress
	}
	d.last, d.hasProgress, d.advances = last, true, d.advances+1
	return nil
}
func (d *applicationDependencies) ExtractTextUnit(_ context.Context, _ string, unit TextUnitInput) (Result, error) {
	d.extracted = append(d.extracted, unit.ID)
	if d.extractErr != nil {
		return Result{}, d.extractErr
	}
	return Result{Entities: []submission.Entity{{Content: knowledge.EntityContent{Identity: knowledge.EntityIdentity{Title: unit.ID, Type: "TEXT_UNIT"}, Description: unit.Text}}}}, nil
}
func (d *applicationDependencies) Submit(_ context.Context, command submission.Command) (submission.Result, error) {
	d.submitted = append(d.submitted, command)
	return submission.Result{SourceID: command.Source.ID}, nil
}
func (d *applicationDependencies) CurrentManifest(context.Context) (knowledge.Manifest, error) {
	return d.versions, nil
}
func (d *applicationDependencies) Publish(_ context.Context, _ []journal.Envelope[knowledgeevents.Body]) error {
	d.published++
	return nil
}
