package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	knowledgesqlite "github.com/memoria-space/meking/knowledge/adapter/sqlite"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	_ "modernc.org/sqlite"
)

type evidenceVerifier struct{}

func (evidenceVerifier) VerifyEvidence(context.Context, []provenance.Evidence) error {
	return nil
}

func TestEntityConflictWorkflow(t *testing.T) {
	database, transactionScope, ctx := openKnowledgeDatabase(t)
	candidates := database.Candidates()
	submissions, err := submission.New(submission.Dependencies{
		Tx:                  transactionScope,
		KnowledgeIdentities: database,
		VersionHistory:      database,
		Sources:             database,
		Candidates:          candidates,
		EvidenceVerifier:    evidenceVerifier{},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolutions, err := resolution.New(resolution.Dependencies{
		Tx:                  transactionScope,
		KnowledgeIdentities: database,
		VersionHistory:      database,
		Candidates:          candidates,
		Provenance:          database,
	})
	if err != nil {
		t.Fatal(err)
	}

	first := submitEntity(t, ctx, submissions, "source-1", "first description")
	if len(first.CreatedVersions.Entities) != 1 || first.CreatedVersions.Entities[0].Version != 1 {
		t.Fatalf("first Submission result = %#v", first)
	}
	entityID := first.CreatedVersions.Entities[0].ID

	identical := submitEntity(t, ctx, submissions, "source-2", "first description")
	if len(identical.ConfirmedVersions.Entities) != 1 ||
		identical.ConfirmedVersions.Entities[0].ID != entityID ||
		identical.ConfirmedVersions.Entities[0].Version != 1 {
		t.Fatalf("identical Submission result = %#v", identical)
	}

	different := submitEntity(t, ctx, submissions, "source-3", "different description")
	if len(different.OpenedCandidates.Entities) != 1 ||
		different.OpenedCandidates.Entities[0].BaseVersion != 1 {
		t.Fatalf("different Submission result = %#v", different)
	}

	conflict, err := resolutions.EntityConflict(ctx, entityID)
	if err != nil {
		t.Fatal(err)
	}
	if !conflict.HasCurrent || conflict.Current.Version != 1 || len(conflict.Candidates) != 1 {
		t.Fatalf("Entity conflict = %#v", conflict)
	}
	expectations := entityExpectations(conflict)
	kept, err := resolutions.ResolveEntity(ctx, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         "resolution-1",
			Kind:       provenance.Resolution,
			ProducerID: "agent-command-1",
		},
		BaseVersion: conflict.BaseVersion,
		Final:       conflict.Current.Knowledge,
		Candidates:  expectations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if kept.CreatedVersion || kept.Version != 1 {
		t.Fatalf("keep-current Resolution result = %#v", kept)
	}
	if _, err := resolutions.EntityConflict(ctx, entityID); !errors.Is(err, knowledge.ErrNotFound) {
		t.Fatalf("resolved Entity conflict error = %v", err)
	}

	retriggered := submitEntity(t, ctx, submissions, "source-4", "different description")
	if len(retriggered.OpenedCandidates.Entities) != 1 {
		t.Fatalf("retriggered Submission result = %#v", retriggered)
	}
	conflict, err = resolutions.EntityConflict(ctx, entityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflict.Candidates) != 1 || len(conflict.Candidates[0].Sources) != 1 {
		t.Fatalf("retriggered Entity conflict = %#v", conflict)
	}
	changed, err := resolutions.ResolveEntity(ctx, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         "resolution-2",
			Kind:       provenance.Resolution,
			ProducerID: "agent-command-2",
		},
		BaseVersion: conflict.BaseVersion,
		Final:       conflict.Candidates[0].Candidate.Content,
		Candidates:  entityExpectations(conflict),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed.CreatedVersion || changed.Version != 2 {
		t.Fatalf("changed Resolution result = %#v", changed)
	}
	current, found, err := database.CurrentEntity(ctx, entityID)
	if err != nil || !found || current.Version != 2 || current.Knowledge.Description != "different description" {
		t.Fatalf("Current Entity = %#v, found = %v, error = %v", current, found, err)
	}
}

func TestNewIdentityWithDifferentContentsCreatesFormalBase(t *testing.T) {
	database, transactionScope, ctx := openKnowledgeDatabase(t)
	candidates := database.Candidates()
	submissions, err := submission.New(submission.Dependencies{
		Tx:                  transactionScope,
		KnowledgeIdentities: database,
		VersionHistory:      database,
		Sources:             database,
		Candidates:          candidates,
		EvidenceVerifier:    evidenceVerifier{},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolutions, err := resolution.New(resolution.Dependencies{
		Tx:                  transactionScope,
		KnowledgeIdentities: database,
		VersionHistory:      database,
		Candidates:          candidates,
		Provenance:          database,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := submissions.Submit(ctx, submission.Command{
		Source: provenance.Source{
			ID:         "source-new-conflict",
			Kind:       provenance.Agent,
			ProducerID: "agent-command-new-conflict",
		},
		Entities: []submission.Entity{
			{
				Content: knowledge.EntityContent{
					Identity: knowledge.EntityIdentity{
						Title: "ENTITY",
						Type:  "ORGANIZATION",
					},
					Description: "first description",
				},
			},
			{
				Content: knowledge.EntityContent{
					Identity: knowledge.EntityIdentity{
						Title: "ENTITY",
						Type:  "ORGANIZATION",
					},
					Description: "second description",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CreatedVersions.Entities) != 1 || len(result.OpenedCandidates.Entities) != 1 {
		t.Fatalf("new identity Submission result = %#v", result)
	}
	entityID := result.CreatedVersions.Entities[0].ID
	for _, candidate := range result.OpenedCandidates.Entities {
		if candidate.ID != entityID || candidate.BaseVersion != 1 {
			t.Fatalf("new identity Candidate = %#v", candidate)
		}
	}
	if current, found, err := database.CurrentEntity(ctx, entityID); err != nil || !found || current.Version != 1 {
		t.Fatalf("new identity Current = %#v, found = %v, error = %v", current, found, err)
	}
	conflict, err := resolutions.EntityConflict(ctx, entityID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := resolutions.ResolveEntity(ctx, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         "resolution-new-conflict",
			Kind:       provenance.Resolution,
			ProducerID: "agent-resolution-new-conflict",
		},
		BaseVersion: 1,
		Final:       conflict.Candidates[0].Candidate.Content,
		Candidates:  entityExpectations(conflict),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.CreatedVersion || created.Version != 2 {
		t.Fatalf("new identity Resolution result = %#v", created)
	}
}

func submitEntity(
	t *testing.T,
	ctx context.Context,
	application *submission.Application,
	sourceID string,
	description string,
) submission.Result {
	t.Helper()
	result, err := application.Submit(ctx, submission.Command{
		Source: provenance.Source{
			ID:         sourceID,
			Kind:       provenance.Agent,
			ProducerID: sourceID,
		},
		Entities: []submission.Entity{
			{
				Content: knowledge.EntityContent{
					Identity: knowledge.EntityIdentity{
						Title: "ENTITY",
						Type:  "ORGANIZATION",
					},
					Description: description,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func entityExpectations(conflict resolution.EntityConflict) []resolution.EntityExpectation {
	result := make([]resolution.EntityExpectation, 0, len(conflict.Candidates))
	for _, candidate := range conflict.Candidates {
		sourceIDs := make([]string, 0, len(candidate.Sources))
		for _, source := range candidate.Sources {
			sourceIDs = append(sourceIDs, source.SourceID)
		}
		if len(sourceIDs) > 0 {
			result = append(result, resolution.EntityExpectation{
				Candidate: candidate.Candidate.Key,
				SourceIDs: sourceIDs,
			})
		}
	}
	return result
}

func openKnowledgeDatabase(
	t *testing.T,
) (*knowledgesqlite.Database, *transactionsqlite.Tx, context.Context) {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "knowledge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	for _, statement := range []string{
		"PRAGMA foreign_keys = OFF",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = FULL",
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	knowledgeDatabase, err := knowledgesqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	transactionScope, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	zoneID, err := zone.NewID()
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := zone.NewContext(t.Context(), zoneID)
	if err != nil {
		t.Fatal(err)
	}
	return knowledgeDatabase, transactionScope, ctx
}
