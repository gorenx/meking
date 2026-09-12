package sqlite_test

import (
	"errors"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
)

const (
	reportOne = communityreport.ID("00000000-0000-4000-8000-000000000001")
	reportTwo = communityreport.ID("00000000-0000-4000-8000-000000000002")
	claimA    = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
)

func TestReportArchiveRoundTripsOrderedSources(t *testing.T) {
	store, _ := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	archive, err := communityreport.NewArchive(
		store,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("NewArchive: %v", err)
	}
	report := persistedReport(t, reportOne)
	if err := archive.Save(communityTestContext(t), []communityreport.Report{report}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := archive.Report(communityTestContext(t), report.ID)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !reflect.DeepEqual(got, report) {
		t.Fatalf("Report = %#v, want %#v", got, report)
	}
	second := persistedReport(t, reportTwo)
	if err := archive.Save(communityTestContext(t), []communityreport.Report{second}); err != nil {
		t.Fatalf("Save second Report: %v", err)
	}
	batch, err := archive.Reports(
		communityTestContext(t),
		[]communityreport.ID{second.ID, report.ID},
	)
	if err != nil {
		t.Fatalf("Reports: %v", err)
	}
	if !reflect.DeepEqual(batch, []communityreport.Report{second, report}) {
		t.Fatalf("Reports = %#v", batch)
	}
	if err := archive.Save(
		communityTestContext(t),
		[]communityreport.Report{report},
	); !errors.Is(err, communityreport.ErrReportConflict) {
		t.Fatalf("duplicate Save error = %v, want ErrReportConflict", err)
	}
}

func TestReportBatchRollsBackWhenLaterIdentityConflicts(t *testing.T) {
	store, _ := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	archive, err := communityreport.NewArchive(
		store,
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatalf("NewArchive: %v", err)
	}
	existing := persistedReport(t, reportOne)
	if err := archive.Save(
		communityTestContext(t),
		[]communityreport.Report{existing},
	); err != nil {
		t.Fatalf("save existing Report: %v", err)
	}
	candidate := persistedReport(t, reportTwo)
	if err := archive.Save(
		communityTestContext(t),
		[]communityreport.Report{candidate, existing},
	); !errors.Is(err, communityreport.ErrReportConflict) {
		t.Fatalf("conflicting batch error = %v, want ErrReportConflict", err)
	}
	if _, err := archive.Report(
		communityTestContext(t),
		candidate.ID,
	); !errors.Is(err, communityreport.ErrReportNotFound) {
		t.Fatalf("rolled-back Report read error = %v, want ErrReportNotFound", err)
	}
}

func persistedReport(t *testing.T, id communityreport.ID) communityreport.Report {
	t.Helper()
	hierarchy, entities, relations := communitySetInput()
	set, err := community.NewCommunitySet(
		hierarchy,
		entities,
		relations,
		community.DefaultDetectConfig(),
	)
	if err != nil {
		t.Fatalf("NewCommunitySet: %v", err)
	}
	draft := community.ReportDraft{
		Title:   "Report",
		Summary: "Summary",
		Findings: []community.ReportFinding{{
			Summary:     "Finding",
			Explanation: "Explanation",
		}},
		Rating:            1.5,
		RatingExplanation: "Rating explanation",
	}
	fullJSON, err := community.MarshalReportJSON(draft)
	if err != nil {
		t.Fatalf("MarshalReportJSON: %v", err)
	}
	return communityreport.Report{
		ID:                id,
		CommunityID:       set.Communities[0].ID,
		Period:            "2026-07-24",
		Title:             draft.Title,
		Summary:           draft.Summary,
		Findings:          append([]community.ReportFinding(nil), draft.Findings...),
		Rank:              draft.Rating,
		RatingExplanation: draft.RatingExplanation,
		FullContent:       community.RenderFullReport(draft),
		FullContentJSON:   fullJSON,
		EntitySources: []community.EntityReference{
			{ID: entityB, Version: 3},
			{ID: entityA, Version: 2},
		},
		RelationSources: []community.RelationReference{{
			ID: relation, Version: 5,
		}},
		ClaimSources: []communityreport.ClaimSource{{
			ID: claimA, Version: 6, EvidenceIndex: 0,
		}},
		TextUnitIDs: []string{strings.Repeat("a", 128)},
		Settings: communityreport.Settings{
			Model:           "report-model",
			PromptHash:      [32]byte{1},
			Tokenizer:       "o200k_base",
			MaxInputTokens:  8_000,
			MaxReportLength: 2_000,
		},
	}
}
