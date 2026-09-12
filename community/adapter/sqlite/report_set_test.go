package sqlite_test

import (
	"errors"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/community"
	communitysqlite "github.com/memoria-space/meking/community/adapter/sqlite"
	communityreport "github.com/memoria-space/meking/community/report"
)

const (
	reportThree      = communityreport.ID("00000000-0000-4000-8000-000000000003")
	fixtureCorporaID = "30000000-0000-4000-8000-000000000001"
)

func TestReportSetStorePersistsImmutableSet(t *testing.T) {
	store, _ := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	communitySet := persistCommunitySet(t, store)
	reports := saveReportSetReports(t, store, communitySet)
	available := map[string]struct{}{strings.Repeat("a", 128): {}}

	first, err := communityreport.NewReportSet(
		communitySet.ID,
		fixtureCorporaID,
		reports,
		available,
	)
	if err != nil {
		t.Fatalf("NewReportSet first: %v", err)
	}
	if err := store.SaveReportSet(communityTestContext(t), first); err != nil {
		t.Fatalf("save first ReportSet: %v", err)
	}
	persisted, err := store.LoadReportSet(communityTestContext(t), first.ID)
	if err != nil || !reflect.DeepEqual(persisted, first) {
		t.Fatalf("stored ReportSet = (%#v, %v), want %#v", persisted, err, first)
	}

	second, err := communityreport.NewReportSet(
		communitySet.ID,
		fixtureCorporaID,
		reports,
		available,
	)
	if err != nil {
		t.Fatalf("NewReportSet second: %v", err)
	}
	if err := store.SaveReportSet(communityTestContext(t), second); err != nil {
		t.Fatalf("save second ReportSet: %v", err)
	}
	if historical, err := store.LoadReportSet(communityTestContext(t), first.ID); err != nil ||
		!reflect.DeepEqual(historical, first) {
		t.Fatalf("historical ReportSet = %#v, %v, want %#v", historical, err, first)
	}
}

func TestReportSetStoreRejectsMissingReportBeforePublication(t *testing.T) {
	store, _ := openCommunityStore(
		t,
		filepath.Join(t.TempDir(), "community.sqlite"),
	)
	communitySet := persistCommunitySet(t, store)
	reports := reportSetReports(t, communitySet)
	archive, err := communityreport.NewArchive(store, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewArchive: %v", err)
	}
	if err := archive.Save(communityTestContext(t), reports[:len(reports)-1]); err != nil {
		t.Fatalf("save partial Report archive: %v", err)
	}
	set, err := communityreport.NewReportSet(
		communitySet.ID,
		fixtureCorporaID,
		reports,
		map[string]struct{}{strings.Repeat("a", 128): {}},
	)
	if err != nil {
		t.Fatalf("NewReportSet: %v", err)
	}
	if err := store.SaveReportSet(
		communityTestContext(t),
		set,
	); !errors.Is(err, communityreport.ErrInvalidReportSet) {
		t.Fatalf("Save ReportSet error = %v, want ErrInvalidReportSet", err)
	}
}

func saveReportSetReports(
	t *testing.T,
	store *communitysqlite.Store,
	set community.CommunitySet,
) []communityreport.Report {
	t.Helper()
	reports := reportSetReports(t, set)
	archive, err := communityreport.NewArchive(store, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewArchive: %v", err)
	}
	if err := archive.Save(communityTestContext(t), reports); err != nil {
		t.Fatalf("Save Reports: %v", err)
	}
	return reports
}

func reportSetReports(
	t *testing.T,
	set community.CommunitySet,
) []communityreport.Report {
	t.Helper()
	ids := []communityreport.ID{reportOne, reportTwo, reportThree}
	reports := make([]communityreport.Report, len(set.Communities))
	for index, current := range set.Communities {
		reports[index] = persistedReport(t, ids[index])
		reports[index].CommunityID = current.ID
	}
	return reports
}
