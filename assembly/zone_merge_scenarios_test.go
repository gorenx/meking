package assembly_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/memoria-space/meking/assembly"
	"github.com/memoria-space/meking/corpus"
	corpuszonemerger "github.com/memoria-space/meking/corpus/zonemerger"
	epochevents "github.com/memoria-space/meking/epoch/integration"
	"github.com/memoria-space/meking/journal"
	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	"github.com/memoria-space/meking/zone"
	zonemerger "github.com/memoria-space/meking/zone/merger"
)

func TestZoneMergePublishesChildOnlyParent(t *testing.T) {
	root := t.TempDir()
	models := newAssemblyModelServer(t)
	config := serviceConfigWithModelURL(t, root, models.URL)
	enableZoneMergeClaims(t, root)
	service, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close assembly Service: %v", err)
		}
	})
	startZoneMergeDispatcher(t, service)

	parent, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext := bindZoneContext(t, parent.ID)
	childContext := bindZoneContext(t, child.ID)
	childPublished := indexZoneDocument(
		t, service, childContext, "zone-merge/child-only-source", "child.txt",
		"CHILD-BETA: Alpha works with Beta.", 0,
	)
	childCorpora, err := service.Documents().Corpora(
		childContext, corpus.CorporaID(childPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	mergeChildIntoParent(t, service, parentContext, child.ID, childPublished)

	const correlation = journal.CorrelationID("zone-merge/child-only-parent")
	const startEvent = journal.EventID("zone-merge/child-only-parent/start")
	startedAt := time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)
	if err := service.CorpusMerger().Prepare(parentContext, corpuszonemerger.Preparation{
		EventID: corpus.EventID(startEvent), CorrelationID: corpus.CorrelationID(correlation),
		OccurredAt: startedAt,
	}); err != nil {
		t.Fatal(err)
	}
	parentPublished := waitForZoneEvent[epochevents.PublishedV1](
		t, service, parent.ID, correlation,
	)
	parentCorpora, err := service.Documents().Corpora(
		parentContext, corpus.CorporaID(parentPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertChildCorporaReused(t, childCorpora, parentCorpora)
	parentEntities, err := service.Knowledge().BrowseEntities(
		parentContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEntityTitles(t, parentEntities.Items, "ALPHA", "BETA")
	parentRelations, err := service.Knowledge().BrowseRelations(
		parentContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertRelationTitles(t, parentEntities.Items, parentRelations.Items, "ALPHA", "BETA")
	parentClaims, err := service.Knowledge().BrowseClaims(
		parentContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(parentClaims.Items) == 0 {
		t.Fatal("Child-only Parent Knowledge contains no Claim")
	}
	assertNoLocalCorpusEvents(t, service, parent.ID, correlation)
	assertNoChildMergeEvents(t, service, parent.ID, correlation)
}

func TestZoneMergeCombinesMultipleChildrenAndKeepsSiblingPublicationsIsolated(t *testing.T) {
	service := openZoneMergeScenarioService(t)
	parent, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	childBeta, err := service.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	childDelta, err := service.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext := bindZoneContext(t, parent.ID)
	betaContext := bindZoneContext(t, childBeta.ID)
	deltaContext := bindZoneContext(t, childDelta.ID)
	betaPublished := indexZoneDocument(
		t, service, betaContext, "zone-merge/multi-beta", "beta.txt",
		"CHILD-BETA: Alpha works with Beta.", 0,
	)
	deltaPublished := indexZoneDocument(
		t, service, deltaContext, "zone-merge/multi-delta", "delta.txt",
		"CHILD-DELTA: Alpha works with Delta.", 0,
	)
	betaEntities, err := service.Knowledge().BrowseEntities(
		betaContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	deltaEntities, err := service.Knowledge().BrowseEntities(
		deltaContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEntityTitles(t, betaEntities.Items, "ALPHA", "BETA")
	assertEntityTitleAbsent(t, betaEntities.Items, "DELTA")
	assertEntityTitles(t, deltaEntities.Items, "ALPHA", "DELTA")
	assertEntityTitleAbsent(t, deltaEntities.Items, "BETA")

	mergeChildIntoParent(t, service, parentContext, childBeta.ID, betaPublished)
	mergeChildIntoParent(t, service, parentContext, childDelta.ID, deltaPublished)
	const parentCorrelation = "zone-merge/multi-parent"
	parentPublished := indexZoneDocument(
		t, service, parentContext, parentCorrelation, "parent.txt",
		"PARENT-GAMMA: Alpha works with Gamma.", 0,
	)
	parentEntities, err := service.Knowledge().BrowseEntities(
		parentContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEntityTitles(t, parentEntities.Items, "ALPHA", "BETA", "DELTA", "GAMMA")
	parentRelations, err := service.Knowledge().BrowseRelations(
		parentContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertRelationTitles(t, parentEntities.Items, parentRelations.Items, "ALPHA", "BETA")
	assertRelationTitles(t, parentEntities.Items, parentRelations.Items, "ALPHA", "DELTA")
	assertRelationTitles(t, parentEntities.Items, parentRelations.Items, "ALPHA", "GAMMA")

	parentCorpora, err := service.Documents().Corpora(
		parentContext, corpus.CorporaID(parentPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct {
		ctx       context.Context
		published epochevents.PublishedV1
	}{{betaContext, betaPublished}, {deltaContext, deltaPublished}} {
		childCorpora, err := service.Documents().Corpora(
			source.ctx, corpus.CorporaID(source.published.CorporaID),
		)
		if err != nil {
			t.Fatal(err)
		}
		assertChildCorporaReused(t, childCorpora, parentCorpora)
	}
	assertNoChildMergeEvents(t, service, parent.ID, journal.CorrelationID(parentCorrelation))
}

func TestZoneMergeUsesLatestPublishedChildKnowledge(t *testing.T) {
	service := openZoneMergeScenarioService(t)
	parent, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext := bindZoneContext(t, parent.ID)
	childContext := bindZoneContext(t, child.ID)
	first := indexZoneDocument(
		t, service, childContext, "zone-merge/advance-child-1", "child-one.txt",
		"CHILD-BETA: Alpha works with Beta.", 0,
	)
	mergeChildIntoParent(t, service, parentContext, child.ID, first)
	second := indexZoneDocument(
		t, service, childContext, "zone-merge/advance-child-2", "child-two.txt",
		"CHILD-BETA: Alpha works with Beta using new evidence.", int64(first.EpochID),
	)
	if second.EpochID <= first.EpochID {
		t.Fatalf("second Child EpochID = %d, want newer than %d", second.EpochID, first.EpochID)
	}
	mergeChildIntoParent(t, service, parentContext, child.ID, second)
	parentPublished := indexZoneDocument(
		t, service, parentContext, "zone-merge/advance-parent", "parent.txt",
		"PARENT-GAMMA: Alpha works with Gamma.", 0,
	)
	childCorpora, err := service.Documents().Corpora(childContext, corpus.CorporaID(second.CorporaID))
	if err != nil {
		t.Fatal(err)
	}
	parentCorpora, err := service.Documents().Corpora(parentContext, corpus.CorporaID(parentPublished.CorporaID))
	if err != nil {
		t.Fatal(err)
	}
	assertChildCorporaReused(t, childCorpora, parentCorpora)
}

func TestZoneMergePublishesChildClaimsToParent(t *testing.T) {
	root := t.TempDir()
	models := newAssemblyModelServer(t)
	config := serviceConfigWithModelURL(t, root, models.URL)
	enableZoneMergeClaims(t, root)
	service, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close assembly Service: %v", err)
		}
	})
	startZoneMergeDispatcher(t, service)

	parent, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext := bindZoneContext(t, parent.ID)
	childContext := bindZoneContext(t, child.ID)
	childPublished := indexZoneDocument(
		t, service, childContext, "zone-merge/claim-child", "child.txt",
		"CHILD-BETA: Alpha works with Beta.", 0,
	)
	childClaims, err := service.Knowledge().BrowseClaims(
		childContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(childClaims.Items) == 0 {
		t.Fatal("Child Knowledge contains no Claim")
	}
	childCorpora, err := service.Documents().Corpora(
		childContext, corpus.CorporaID(childPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	childTextUnits := make(map[string]struct{})
	for _, chunkedText := range childCorpora.Texts {
		for _, unit := range chunkedText.TextUnits {
			childTextUnits[string(unit.ID())] = struct{}{}
		}
	}

	mergeChildIntoParent(t, service, parentContext, child.ID, childPublished)
	indexZoneDocument(
		t, service, parentContext, "zone-merge/claim-parent", "parent.txt",
		"PARENT-GAMMA: Alpha works with Gamma.", 0,
	)
	parentClaims, err := service.Knowledge().BrowseClaims(
		parentContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(parentClaims.Items) == 0 {
		t.Fatal("Parent Knowledge contains no Claim")
	}
	childClaimIDs := make(map[string]struct{}, len(childClaims.Items))
	for _, claim := range childClaims.Items {
		childClaimIDs[claim.ID] = struct{}{}
	}
	foundChildEvidence := false
	for _, claim := range parentClaims.Items {
		if _, reused := childClaimIDs[claim.ID]; reused {
			t.Fatalf("Parent reused Child Claim ID %q", claim.ID)
		}
		for _, statement := range claim.Evidence {
			if _, foundChild := childTextUnits[statement.TextUnitID]; foundChild {
				foundChildEvidence = true
			}
		}
	}
	if !foundChildEvidence {
		t.Fatal("Parent Claims contain no Statement backed by a Child TextUnit")
	}
}

func TestZoneMergeRejectsInvalidHierarchyWithoutParentSideEffects(t *testing.T) {
	service := openZoneMergeScenarioService(t)
	owner, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	otherParent, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.Zones().CreateChild(t.Context(), owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Zones().CreateChild(t.Context(), child.ID); !errors.Is(err, zone.ErrChildDepth) {
		t.Fatalf("CreateChild() below Child error = %v, want %v", err, zone.ErrChildDepth)
	}
	otherContext := bindZoneContext(t, otherParent.ID)
	wrongMergeContext, err := zone.WithChildZone(otherContext, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ZoneMerger().Merge(wrongMergeContext, zonemerger.Input{
		SourceCorporaID: "1",
	}); !errors.Is(err, zonemerger.ErrNotDirectParent) {
		t.Fatalf("Merge() through non-Parent error = %v, want %v", err, zonemerger.ErrNotDirectParent)
	}
	if _, err := zone.WithChildZone(otherContext, otherParent.ID); !errors.Is(err, zone.ErrContextConflict) {
		t.Fatalf("WithChildZone() current Zone error = %v, want %v", err, zone.ErrContextConflict)
	}
	indexZoneDocument(
		t, service, otherContext, "zone-merge/invalid-parent", "parent.txt",
		"PARENT-GAMMA: Alpha works with Gamma.", 0,
	)
	entities, err := service.Knowledge().BrowseEntities(
		otherContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEntityTitles(t, entities.Items, "ALPHA", "GAMMA")
	assertEntityTitleAbsent(t, entities.Items, "BETA")
}

func TestZoneMergeRejectsUnreadableChildCorporaWithoutParentSideEffects(t *testing.T) {
	service := openZoneMergeScenarioService(t)
	parent, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext := bindZoneContext(t, parent.ID)
	childContext := bindZoneContext(t, child.ID)
	childPublished := indexZoneDocument(
		t, service, childContext, "zone-merge/stable-child", "child.txt",
		"CHILD-BETA: Alpha works with Beta.", 0,
	)
	mergeContext, err := zone.WithChildZone(parentContext, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	corporaSequence, err := strconv.ParseInt(string(childPublished.CorporaID), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ZoneMerger().Merge(mergeContext, zonemerger.Input{
		SourceCorporaID: strconv.FormatInt(corporaSequence+1, 10),
	}); err == nil {
		t.Fatal("Merge() accepted an unreadable Child Corpora boundary")
	}
	indexZoneDocument(
		t, service, parentContext, "zone-merge/stable-parent", "parent.txt",
		"PARENT-GAMMA: Alpha works with Gamma.", 0,
	)
	entities, err := service.Knowledge().BrowseEntities(
		parentContext, queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEntityTitles(t, entities.Items, "ALPHA", "GAMMA")
	assertEntityTitleAbsent(t, entities.Items, "BETA")
}

func openZoneMergeScenarioService(t *testing.T) *assembly.Service {
	t.Helper()
	root := t.TempDir()
	models := newAssemblyModelServer(t)
	service, err := assembly.Open(t.Context(), serviceConfigWithModelURL(t, root, models.URL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close assembly Service: %v", err)
		}
	})
	startZoneMergeDispatcher(t, service)
	return service
}

func enableZoneMergeClaims(t *testing.T, root string) {
	t.Helper()
	settingsPath := filepath.Join(root, "settings.yaml")
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	current := "        claims:\n            enabled: false\n            prompt: prompts/extract_claims.txt\n            description: Any claims or facts that could be relevant to information discovery.\n            max_gleanings: 1"
	enabled := "        claims:\n            enabled: true\n            prompt: prompts/extract_claims.txt\n            description: Any claims or facts that could be relevant to information discovery.\n            max_gleanings: 0"
	updated := strings.Replace(string(settings), current, enabled, 1)
	if updated == string(settings) {
		t.Fatal("default Claim settings were not found")
	}
	if err := os.WriteFile(settingsPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mergeChildIntoParent(
	t *testing.T,
	service *assembly.Service,
	parentContext context.Context,
	childID zone.ID,
	published epochevents.PublishedV1,
) {
	t.Helper()
	ctx, err := zone.WithChildZone(parentContext, childID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ZoneMerger().Merge(ctx, zonemerger.Input{
		SourceCorporaID: string(published.CorporaID),
	}); err != nil {
		t.Fatal(err)
	}
}

func assertEntityTitleAbsent(t *testing.T, entities []queryknowledge.Entity, title string) {
	t.Helper()
	for _, entity := range entities {
		if entity.Title == title {
			t.Fatalf("Knowledge Entity title %q is unexpectedly present", title)
		}
	}
}

func assertRelationTitles(
	t *testing.T,
	entities []queryknowledge.Entity,
	relations []queryknowledge.Relation,
	source string,
	target string,
) {
	t.Helper()
	titles := make(map[string]string, len(entities))
	for _, entity := range entities {
		titles[entity.ID] = entity.Title
	}
	for _, relation := range relations {
		if titles[relation.SourceEntityID] == source && titles[relation.TargetEntityID] == target {
			return
		}
	}
	t.Fatalf("Knowledge Relation %s -> %s is missing", source, target)
}

func assertNoChildMergeEvents(
	t *testing.T,
	service *assembly.Service,
	zoneID zone.ID,
	correlationID journal.CorrelationID,
) {
	t.Helper()
	forbidden := map[string]struct{}{
		"corpus.corpora_activated":       {},
		"corpus.text_unit_vectors_ready": {},
	}
	events, err := service.Journal().Entries(bindZoneContext(t, zoneID), 0, 512)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.ZoneID != zoneID || event.CorrelationID != correlationID {
			continue
		}
		eventType := strings.ToLower(string(event.Type))
		_, explicitlyForbidden := forbidden[eventType]
		if strings.Contains(eventType, "child") || strings.Contains(eventType, "merge") || explicitlyForbidden {
			t.Fatalf("Parent Journal contains forbidden Zone Merge event type %q", event.Type)
		}
	}
}

func assertNoLocalCorpusEvents(
	t *testing.T,
	service *assembly.Service,
	zoneID zone.ID,
	correlationID journal.CorrelationID,
) {
	t.Helper()
	events, err := service.Journal().Entries(bindZoneContext(t, zoneID), 0, 512)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.ZoneID == zoneID && event.CorrelationID == correlationID &&
			(event.Type == "corpus.document_recorded" || event.Type == "corpus.text_prepared") {
			t.Fatalf("Child-only Parent published local Corpus event %q", event.Type)
		}
	}
}
