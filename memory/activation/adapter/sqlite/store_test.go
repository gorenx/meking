package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/memoria-space/meking/internal/sqlitepool"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/memory/activation"
	activationsqlite "github.com/memoria-space/meking/memory/activation/adapter/sqlite"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

const subjectID = knowledge.EntityID("11111111-1111-4111-8111-111111111111")

type targetReader struct {
	target activation.CurrentTarget
	found  bool
	err    error
}

func (reader *targetReader) ReadCurrent(context.Context, knowledge.ObjectRef) (activation.CurrentTarget, bool, error) {
	return reader.target, reader.found, reader.err
}

type evidenceReader struct{ err error }

func (reader evidenceReader) Validate(context.Context, []activation.Evidence) error {
	return reader.err
}

type fixture struct {
	database *sql.DB
	store    *activationsqlite.Database
	tx       *transactionsqlite.Tx
	service  *activation.Service
	targets  *targetReader
	model    activation.Model
	protocol activation.Protocol
	ctx      context.Context
	path     string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{path: filepath.Join(t.TempDir(), "project.sqlite")}
	f.open(t)
	t.Cleanup(func() {
		if f.database != nil {
			if err := f.database.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	ctx, err := zone.NewContext(context.Background(), zone.ID("22222222-2222-4222-8222-222222222222"))
	if err != nil {
		t.Fatal(err)
	}
	f.ctx = ctx
	f.targets = &targetReader{target: activation.CurrentTarget{Version: 1}, found: true}
	f.model, err = activation.NewModel(mas.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	f.protocol, err = activation.NewProtocol("1", "en", "Evaluate natural conversational recall.", `{"type":"object"}`, `{"type":"object"}`)
	if err != nil {
		t.Fatal(err)
	}
	f.archive(t, f.model, f.protocol)
	f.service = f.newService(t, f.store, f.model, []activation.Protocol{f.protocol}, evidenceReader{})
	return f
}

func (f *fixture) open(t *testing.T) {
	t.Helper()
	var err error
	f.database, err = sqlitepool.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store, err = activationsqlite.New(f.database)
	if err != nil {
		t.Fatal(err)
	}
	f.tx, err = transactionsqlite.New(f.database)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) archive(t *testing.T, model activation.Model, protocols ...activation.Protocol) {
	t.Helper()
	if len(protocols) == 0 {
		protocols = []activation.Protocol{f.protocol}
	}
	if err := activation.Prepare(context.Background(), f.tx, f.store, model, protocols); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) newService(t *testing.T, store activation.Store, model activation.Model, protocols []activation.Protocol, evidence activation.EvidenceReader) *activation.Service {
	t.Helper()
	service, err := activation.NewService(activation.Dependencies{Tx: f.tx, Store: store, Targets: f.targets, Evidence: evidence, Protocols: protocols, Model: model, Now: func() time.Time { return time.Date(2026, 9, 12, 18, 0, 0, 123456789, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func (f *fixture) input(id string, at time.Time) activation.Observation {
	grade := mas.GradeGood
	return activation.Observation{ID: id, Subject: subjectID, Version: 1, OccurredAt: at, RecallActorID: "actor", EvaluatorID: "evaluator", ProtocolVersion: f.protocol.Version(), ProtocolDigest: f.protocol.Digest(),
		Evidence:   []activation.Evidence{{Ref: "expression", SpeakerID: "actor", CompletedAt: at, Source: activation.Inline{HostRecordID: "host-" + id, Role: "assistant", Text: "Spontaneous conversational recall."}, Recall: &activation.AvailableInformation{Coverage: "complete"}}},
		Assessment: activation.Assessment{Status: "graded", Grade: &grade, Rationale: "Recall succeeded with observable ordinary effort.", EvidenceRefs: []string{"expression"}}}
}

func (f *fixture) state(t *testing.T) activation.StoredState {
	t.Helper()
	state, found, err := f.service.ReadState(f.ctx, subjectID)
	if err != nil || !found {
		t.Fatalf("ReadState: %v %v", found, err)
	}
	return state
}

func TestSubmitPersistsOnlyOnceAndReturnsOriginalReceipt(t *testing.T) {
	f := setup(t)
	at := time.Date(2026, 9, 10, 10, 0, 0, 987654321, time.UTC)
	input := f.input("first", at)
	receipt, err := f.service.Submit(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != "applied" || receipt.AppliedSequence != 1 || receipt.Duplicate {
		t.Fatalf("receipt: %#v", receipt)
	}
	scheduler, _ := mas.NewScheduler(f.model.Config())
	want, err := scheduler.UpdateLiveness(mas.State{}, mas.GradeGood, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.state(t); got.State != want || got.Sequence != 1 {
		t.Fatalf("first calculation: %#v", got)
	}
	second := f.input("second", at.Add(24*time.Hour))
	if _, err := f.service.Submit(f.ctx, second); err != nil {
		t.Fatal(err)
	}
	f.targets.target.Version = 2
	retried, err := f.service.Submit(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !retried.Duplicate {
		t.Fatal("retry did not report duplicate")
	}
	retried.Duplicate = false
	if retried != receipt {
		t.Fatalf("original receipt changed: %#v != %#v", retried, receipt)
	}
	if f.state(t).Sequence != 2 {
		t.Fatal("duplicate advanced state")
	}
	input.Assessment.Rationale = "different"
	if _, err := f.service.Submit(f.ctx, input); !errors.Is(err, activation.ErrObservationConflict) {
		t.Fatalf("conflicting retry: %v", err)
	}
}

func TestUnscorableDoesNotInitializeOrChangeState(t *testing.T) {
	f := setup(t)
	at := time.Now().UTC()
	input := f.input("unscorable", at)
	input.Assessment.Status = "unscorable"
	input.Assessment.Grade = nil
	input.Assessment.ReasonCode = "difficulty_unknown"
	receipt, err := f.service.Submit(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != "recorded_unscorable" || receipt.AppliedSequence != 0 {
		t.Fatalf("receipt: %#v", receipt)
	}
	if _, found, err := f.service.ReadState(f.ctx, subjectID); err != nil || found {
		t.Fatalf("unscorable initialized state: %v %v", found, err)
	}
	if _, err := f.service.Submit(f.ctx, f.input("graded", at)); err != nil {
		t.Fatal(err)
	}
	before := f.state(t)
	input.ID = "another-unscorable"
	if _, err := f.service.Submit(f.ctx, input); err != nil {
		t.Fatal(err)
	}
	if got := f.state(t); got != before {
		t.Fatalf("unscorable changed state: %#v", got)
	}
}

func TestAdmissionRejectsWithoutWrites(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*fixture, *activation.Observation)
		want      error
	}{
		{"version", func(f *fixture, o *activation.Observation) { f.targets.target.Version = 2 }, activation.ErrVersionChanged},
		{"deleted", func(f *fixture, o *activation.Observation) { f.targets.target.Deleted = true }, activation.ErrTargetNotFound},
		{"missing", func(f *fixture, o *activation.Observation) { f.targets.found = false }, activation.ErrTargetNotFound},
		{"protocol", func(f *fixture, o *activation.Observation) { o.ProtocolDigest = "unknown" }, activation.ErrProtocolUnsupported},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := setup(t)
			input := f.input("rejected", time.Now().UTC())
			test.configure(f, &input)
			if _, err := f.service.Submit(f.ctx, input); !errors.Is(err, test.want) {
				t.Fatalf("Submit: %v", err)
			}
			if _, found, err := f.store.FindObservation(f.ctx, input.ID); err != nil || found {
				t.Fatalf("rejected observation persisted: %v %v", found, err)
			}
		})
	}
	f := setup(t)
	input := f.input("no-zone", time.Now().UTC())
	if _, err := f.service.Submit(context.Background(), input); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("missing Zone: %v", err)
	}
	injected := errors.New("evidence unavailable")
	service := f.newService(t, f.store, f.model, []activation.Protocol{f.protocol}, evidenceReader{err: injected})
	if _, err := service.Submit(f.ctx, input); !errors.Is(err, injected) {
		t.Fatalf("evidence failure: %v", err)
	}
}

func TestStateSurvivesVersionChangeAndRejectsBackdatingOrModelMixing(t *testing.T) {
	f := setup(t)
	at := time.Now().UTC()
	first := f.input("first", at)
	if _, err := f.service.Submit(f.ctx, first); err != nil {
		t.Fatal(err)
	}
	f.targets.target.Version = 2
	next := f.input("next", at)
	next.Version = 2
	if receipt, err := f.service.Submit(f.ctx, next); err != nil || receipt.AppliedSequence != 2 {
		t.Fatalf("version reset: %#v %v", receipt, err)
	}
	old := f.input("backdated", at.Add(-time.Nanosecond))
	old.Version = 2
	if _, err := f.service.Submit(f.ctx, old); !errors.Is(err, activation.ErrOutOfOrder) {
		t.Fatalf("backdating: %v", err)
	}
	config := f.model.Config()
	config.Parameters[0] += .01
	model, err := activation.NewModel(config)
	if err != nil {
		t.Fatal(err)
	}
	f.archive(t, model)
	service := f.newService(t, f.store, model, []activation.Protocol{f.protocol}, evidenceReader{})
	next.ID = "new-model"
	if _, err := service.Submit(f.ctx, next); !errors.Is(err, activation.ErrModelMismatch) {
		t.Fatalf("mixed model: %v", err)
	}
	if receipt, err := service.Submit(f.ctx, first); err != nil || !receipt.Duplicate {
		t.Fatalf("retry after model/version change: %#v %v", receipt, err)
	}
}

type failingStore struct {
	activation.Store
	stage   string
	failure error
}

func (store failingStore) InsertObservation(ctx context.Context, record activation.StoredObservation) error {
	if err := store.Store.InsertObservation(ctx, record); err != nil {
		return err
	}
	if store.stage == "observation" {
		return store.failure
	}
	return nil
}
func (store failingStore) SaveState(ctx context.Context, state activation.StoredState) error {
	if err := store.Store.SaveState(ctx, state); err != nil {
		return err
	}
	return store.failure
}

func TestWriteFailuresRollBackWholeApplication(t *testing.T) {
	for _, stage := range []string{"observation", "state"} {
		t.Run(stage, func(t *testing.T) {
			f := setup(t)
			injected := errors.New("injected write failure")
			service := f.newService(t, failingStore{Store: f.store, stage: stage, failure: injected}, f.model, []activation.Protocol{f.protocol}, evidenceReader{})
			input := f.input("rollback", time.Now().UTC())
			if _, err := service.Submit(f.ctx, input); !errors.Is(err, injected) {
				t.Fatalf("Submit: %v", err)
			}
			if _, found, err := f.store.FindObservation(f.ctx, input.ID); err != nil || found {
				t.Fatalf("observation survived rollback: %v %v", found, err)
			}
			if _, found, err := f.store.ReadState(f.ctx, subjectID); err != nil || found {
				t.Fatalf("state survived rollback: %v %v", found, err)
			}
			if receipt, err := f.service.Submit(f.ctx, input); err != nil || receipt.AppliedSequence != 1 {
				t.Fatalf("retry after rollback: %#v %v", receipt, err)
			}
		})
	}
}

func TestConcurrentSubmissionsApplyExactlyOnce(t *testing.T) {
	for _, sameID := range []bool{true, false} {
		t.Run(fmt.Sprintf("same-id-%t", sameID), func(t *testing.T) {
			f := setup(t)
			at := time.Now().UTC()
			const count = 16
			var group sync.WaitGroup
			results := make(chan error, count)
			receipts := make(chan activation.Receipt, count)
			for index := range count {
				group.Go(func() {
					id := "same"
					if !sameID {
						id = fmt.Sprintf("recall-%d", index)
					}
					r, err := f.service.Submit(f.ctx, f.input(id, at))
					results <- err
					receipts <- r
				})
			}
			group.Wait()
			close(results)
			close(receipts)
			for err := range results {
				if err != nil {
					t.Fatal(err)
				}
			}
			applied := 0
			for r := range receipts {
				if !r.Duplicate {
					applied++
				}
			}
			want := count
			if sameID {
				want = 1
			}
			if applied != want || f.state(t).Sequence != uint64(want) {
				t.Fatalf("applied=%d state=%#v", applied, f.state(t))
			}
			f.assertReplay(t)
		})
	}
}

func (f *fixture) assertReplay(t *testing.T) {
	t.Helper()
	records, err := f.store.AppliedObservations(f.ctx, subjectID)
	if err != nil {
		t.Fatal(err)
	}
	model, found, err := f.store.ReadModel(f.ctx, f.model.ID())
	if err != nil || !found {
		t.Fatalf("model: %v %v", found, err)
	}
	replayed, found, err := activation.Replay(model, records)
	if err != nil || !found {
		t.Fatalf("replay: %v %v", found, err)
	}
	if got := f.state(t); got != replayed {
		t.Fatalf("replay differs: %#v != %#v", replayed, got)
	}
	if len(records) > 1 {
		if _, _, err := activation.Replay(model, records[1:]); !errors.Is(err, activation.ErrDataIntegrity) {
			t.Fatalf("missing sequence accepted: %v", err)
		}
	}
}

func TestRestartAndFullRangeTimeRoundTrip(t *testing.T) {
	f := setup(t)
	for index, at := range []time.Time{time.Date(1, 1, 2, 0, 0, 0, 1, time.UTC), time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)} {
		input := f.input(fmt.Sprintf("recall-%d", index), at)
		if _, err := f.service.Submit(f.ctx, input); err != nil {
			t.Fatal(err)
		}
		record, found, err := f.store.FindObservation(f.ctx, input.ID)
		if err != nil || !found {
			t.Fatalf("find: %v %v", found, err)
		}
		want, err := input.Normalize()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, record.Input) {
			t.Fatal("observation lost content or time precision")
		}
	}
	before := f.state(t)
	if err := f.database.Close(); err != nil {
		t.Fatal(err)
	}
	f.database = nil
	f.open(t)
	f.service = f.newService(t, f.store, f.model, []activation.Protocol{f.protocol}, evidenceReader{})
	if after := f.state(t); before != after {
		t.Fatal("state changed across restart")
	}
	f.assertReplay(t)
}

func TestZoneIsolationAndTransactionRequired(t *testing.T) {
	f := setup(t)
	input := f.input("identity", time.Now().UTC())
	receipt, err := f.service.Submit(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	other, err := zone.NewContext(context.Background(), zone.ID("33333333-3333-4333-8333-333333333333"))
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := f.store.FindObservation(other, input.ID); err != nil || found {
		t.Fatalf("cross-Zone observation: %v %v", found, err)
	}
	if _, found, err := f.service.ReadState(other, subjectID); err != nil || found {
		t.Fatalf("cross-Zone state: %v %v", found, err)
	}
	if got, err := f.service.Submit(other, input); err != nil || got.Duplicate || got.AppliedSequence != 1 {
		t.Fatalf("separate Zone: %#v %v", got, err)
	}
	if err := f.store.InsertObservation(f.ctx, activation.StoredObservation{Input: input, Receipt: receipt}); !errors.Is(err, transactionsqlite.ErrNoTransaction) {
		t.Fatalf("unscoped write: %v", err)
	}
	if err := f.store.SaveState(f.ctx, f.state(t)); !errors.Is(err, transactionsqlite.ErrNoTransaction) {
		t.Fatalf("unscoped state: %v", err)
	}
	if err := f.store.InsertModel(f.ctx, f.model); !errors.Is(err, transactionsqlite.ErrNoTransaction) {
		t.Fatalf("unscoped archive: %v", err)
	}
}

func TestCorruptPersistenceIsRejected(t *testing.T) {
	for _, test := range []struct {
		name, query string
		read        func(*fixture) error
	}{
		{"time", `UPDATE activation_states SET last_review='broken'`, func(f *fixture) error { _, _, err := f.service.ReadState(f.ctx, subjectID); return err }},
		{"observation-index", `UPDATE activation_observations SET version='2'`, func(f *fixture) error { _, _, err := f.store.FindObservation(f.ctx, "first"); return err }},
		{"model-archive", `UPDATE activation_models SET formula='unknown'`, func(f *fixture) error { _, _, err := f.store.ReadModel(f.ctx, f.model.ID()); return err }},
		{"protocol-archive", `UPDATE activation_protocols SET prompt='changed'`, func(f *fixture) error { _, _, err := f.store.ReadProtocol(f.ctx, f.protocol.Digest()); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := setup(t)
			if _, err := f.service.Submit(f.ctx, f.input("first", time.Now().UTC())); err != nil {
				t.Fatal(err)
			}
			if _, err := f.database.Exec(test.query); err != nil {
				t.Fatal(err)
			}
			if err := test.read(f); !errors.Is(err, activation.ErrDataIntegrity) {
				t.Fatalf("corruption accepted: %v", err)
			}
		})
	}
}

func TestPreparationVerifiesHistoryAndCurrentState(t *testing.T) {
	for _, test := range []struct{ name, query string }{
		{"valid-number-wrong-state", `UPDATE activation_states SET stability=stability+1`},
		{"missing-state", `DELETE FROM activation_states`},
		{"missing-observation", `DELETE FROM activation_observations`},
		{"missing-protocol", `DELETE FROM activation_protocols`},
		{"missing-model", `DELETE FROM activation_models`},
		{"missing-table", `DROP TABLE activation_observations`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := setup(t)
			if _, err := f.service.Submit(f.ctx, f.input("first", time.Now().UTC())); err != nil {
				t.Fatal(err)
			}
			if _, err := f.database.Exec(test.query); err != nil {
				t.Fatal(err)
			}
			if err := activation.Prepare(f.ctx, f.tx, f.store, f.model, []activation.Protocol{f.protocol}); err == nil {
				t.Fatal("corrupt history passed domain preparation")
			}
		})
	}
}

func TestArchiveMustExistBeforeWritingObservation(t *testing.T) {
	f := setup(t)
	input := f.input("archive", time.Now().UTC())
	if _, err := f.database.Exec(`DELETE FROM activation_protocols`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Submit(f.ctx, input); !errors.Is(err, activation.ErrDataIntegrity) {
		t.Fatalf("missing archive accepted: %v", err)
	}
	if _, found, err := f.store.FindObservation(f.ctx, input.ID); err != nil || found {
		t.Fatalf("failed archive check wrote observation: %v %v", found, err)
	}
}
