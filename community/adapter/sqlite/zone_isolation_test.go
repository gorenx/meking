package sqlite_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/community"
	communitysqlite "github.com/memoria-space/meking/community/adapter/sqlite"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/zone"
)

func TestCommunityStoreIsolatesZones(t *testing.T) {
	store, _ := openCommunityStore(t, filepath.Join(t.TempDir(), "community.sqlite"))
	zoneA := communityZoneContext(t, "10000000-0000-4000-8000-000000000001")
	zoneB := communityZoneContext(t, "20000000-0000-4000-8000-000000000002")

	hierarchy, entities, relations := communitySetInput()
	setA, err := community.NewCommunitySet(
		hierarchy, entities, relations, community.DefaultDetectConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	setB := setA
	setB.DetectionConfig.Seed++
	if err := store.Save(zoneA, setA); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(zoneB, setB); err != nil {
		t.Fatal(err)
	}
	loadedA, err := store.Load(zoneA, setA.ID)
	if err != nil || !reflect.DeepEqual(loadedA, setA) {
		t.Fatalf("Zone A CommunitySet = (%#v, %v), want %#v", loadedA, err, setA)
	}
	loadedB, err := store.Load(zoneB, setB.ID)
	if err != nil || !reflect.DeepEqual(loadedB, setB) {
		t.Fatalf("Zone B CommunitySet = (%#v, %v), want %#v", loadedB, err, setB)
	}

	reports := reportSetReports(t, setA)
	if err := store.Insert(zoneA, reports); err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(zoneB, reports); err != nil {
		t.Fatal(err)
	}
	reportSet, err := communityreport.NewReportSet(
		setA.ID,
		fixtureCorporaID,
		reports,
		map[string]struct{}{strings.Repeat("a", 128): {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveReportSet(zoneA, reportSet); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveReportSet(zoneB, reportSet); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(t.Context(), setA.ID); !errors.Is(err, zone.ErrContextRequired) {
		t.Fatalf("unscoped CommunitySet Load error = %v, want %v", err, zone.ErrContextRequired)
	}
}

var _ communityreport.Store = (*communitysqlite.Store)(nil)
