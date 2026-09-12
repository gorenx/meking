package sqlite_test

import (
	"context"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/knowledge/zonemerger"
	"github.com/memoria-space/meking/zone"
)

func TestChildResolvesConflictCreatedWhileMergingIntoParent(t *testing.T) {
	database, transactionScope, initialContext := openKnowledgeDatabase(t)
	candidates := database.Candidates()
	parentZoneID, err := zone.RequireID(initialContext)
	if err != nil {
		t.Fatal(err)
	}
	childZoneID, err := zone.NewID()
	if err != nil {
		t.Fatal(err)
	}
	parentContext, err := zone.RouteContext(t.Context(), parentZoneID)
	if err != nil {
		t.Fatal(err)
	}
	childContext, err := zone.RouteContext(t.Context(), childZoneID)
	if err != nil {
		t.Fatal(err)
	}
	mergeContext, err := zone.WithChildZone(parentContext, childZoneID)
	if err != nil {
		t.Fatal(err)
	}

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
	reader, err := knowledge.NewReader(database)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := provenance.NewReader(database)
	if err != nil {
		t.Fatal(err)
	}
	merger, err := zonemerger.New(zonemerger.Dependencies{
		Tx:               transactionScope,
		Conflicts:        database,
		Provenance:       metadata,
		Identities:       database,
		CurrentVersions:  database,
		CurrentKnowledge: reader,
		Submissions:      submissions,
		Resolutions:      resolutions,
	})
	if err != nil {
		t.Fatal(err)
	}

	childSubmission := submitZoneEntity(
		t,
		childContext,
		submissions,
		"child-source",
		"child description",
	)
	childEntityID := childSubmission.CreatedVersions.Entities[0].ID
	parentSubmission := submitZoneEntity(
		t,
		parentContext,
		submissions,
		"parent-source",
		"parent description",
	)
	parentEntityID := parentSubmission.CreatedVersions.Entities[0].ID

	if err := merger.MergeChildKnowledge(mergeContext); err != nil {
		t.Fatal(err)
	}
	if _, err := resolutions.EntityConflict(parentContext, parentEntityID); err == nil {
		t.Fatal("Parent conflict was opened before Child review")
	}
	childConflict, err := resolutions.EntityConflict(childContext, childEntityID)
	if err != nil {
		t.Fatal(err)
	}
	if childConflict.BaseVersion != 1 ||
		len(childConflict.Candidates) != 1 ||
		childConflict.Candidates[0].Candidate.Content.Description != "parent description" {
		t.Fatalf("Child conflict = %#v", childConflict)
	}
	review, err := merger.EntityReview(childContext, childEntityID, childConflict.BaseVersion)
	if err != nil {
		t.Fatal(err)
	}
	if review.Assignment.ParentEntityID != parentEntityID ||
		review.Parent.Knowledge.Description != "parent description" ||
		len(review.Child.Candidates) != 1 {
		t.Fatalf("Child review = %#v", review)
	}

	result, err := merger.ResolveEntity(childContext, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         "child-resolution",
			Kind:       provenance.Resolution,
			ProducerID: "child-agent-command",
		},
		BaseVersion: childConflict.BaseVersion,
		Final:       childConflict.Current.Knowledge,
		Candidates:  entityExpectations(childConflict),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedVersion || result.Version != 1 {
		t.Fatalf("Child Resolution result = %#v", result)
	}
	childCurrent, found, err := database.CurrentEntity(childContext, childEntityID)
	if err != nil || !found || childCurrent.Version != 1 ||
		childCurrent.Knowledge.Description != "child description" {
		t.Fatalf("Child Current = %#v, found = %v, error = %v", childCurrent, found, err)
	}
	parentCurrent, found, err := database.CurrentEntity(parentContext, parentEntityID)
	if err != nil || !found || parentCurrent.Version != 2 ||
		parentCurrent.Knowledge.Description != "child description" {
		t.Fatalf("Parent Current = %#v, found = %v, error = %v", parentCurrent, found, err)
	}
	if _, err := resolutions.EntityConflict(childContext, childEntityID); err == nil {
		t.Fatal("Child conflict remains unresolved")
	}
	if _, err := resolutions.EntityConflict(parentContext, parentEntityID); err == nil {
		t.Fatal("Parent conflict remains unresolved")
	}
}

func TestLaterChildContinuesReviewAgainstUpdatedParent(t *testing.T) {
	database, transactionScope, initialContext := openKnowledgeDatabase(t)
	candidates := database.Candidates()
	parentZoneID, err := zone.RequireID(initialContext)
	if err != nil {
		t.Fatal(err)
	}
	firstChildZoneID, err := zone.NewID()
	if err != nil {
		t.Fatal(err)
	}
	secondChildZoneID, err := zone.NewID()
	if err != nil {
		t.Fatal(err)
	}
	parentContext, err := zone.RouteContext(t.Context(), parentZoneID)
	if err != nil {
		t.Fatal(err)
	}
	firstChildContext, err := zone.RouteContext(t.Context(), firstChildZoneID)
	if err != nil {
		t.Fatal(err)
	}
	secondChildContext, err := zone.RouteContext(t.Context(), secondChildZoneID)
	if err != nil {
		t.Fatal(err)
	}
	firstMergeContext, err := zone.WithChildZone(parentContext, firstChildZoneID)
	if err != nil {
		t.Fatal(err)
	}
	secondMergeContext, err := zone.WithChildZone(parentContext, secondChildZoneID)
	if err != nil {
		t.Fatal(err)
	}

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
	reader, err := knowledge.NewReader(database)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := provenance.NewReader(database)
	if err != nil {
		t.Fatal(err)
	}
	merger, err := zonemerger.New(zonemerger.Dependencies{
		Tx:               transactionScope,
		Conflicts:        database,
		Provenance:       metadata,
		Identities:       database,
		CurrentVersions:  database,
		CurrentKnowledge: reader,
		Submissions:      submissions,
		Resolutions:      resolutions,
	})
	if err != nil {
		t.Fatal(err)
	}

	parentID := submitZoneEntity(t, parentContext, submissions, "parent", "parent value").CreatedVersions.Entities[0].ID
	firstChildID := submitZoneEntity(t, firstChildContext, submissions, "first-child", "first value").CreatedVersions.Entities[0].ID
	secondChildID := submitZoneEntity(t, secondChildContext, submissions, "second-child", "second value").CreatedVersions.Entities[0].ID

	if err := merger.MergeChildKnowledge(firstMergeContext); err != nil {
		t.Fatal(err)
	}
	if err := merger.MergeChildKnowledge(secondMergeContext); err != nil {
		t.Fatal(err)
	}
	firstConflict, err := resolutions.EntityConflict(firstChildContext, firstChildID)
	if err != nil {
		t.Fatal(err)
	}
	secondConflict, err := resolutions.EntityConflict(secondChildContext, secondChildID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := merger.ResolveEntity(firstChildContext, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         "first-child-resolution",
			Kind:       provenance.Resolution,
			ProducerID: "first-child-agent",
		},
		BaseVersion: firstConflict.BaseVersion,
		Final:       firstConflict.Current.Knowledge,
		Candidates:  entityExpectations(firstConflict),
	}); err != nil {
		t.Fatal(err)
	}
	parentAfterFirst, found, err := database.CurrentEntity(parentContext, parentID)
	if err != nil || !found || parentAfterFirst.Version != 2 || parentAfterFirst.Knowledge.Description != "first value" {
		t.Fatalf("Parent after first Child = %#v, found = %v, error = %v", parentAfterFirst, found, err)
	}

	if _, err := merger.ResolveEntity(secondChildContext, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         "second-child-resolution-1",
			Kind:       provenance.Resolution,
			ProducerID: "second-child-agent-1",
		},
		BaseVersion: secondConflict.BaseVersion,
		Final:       secondConflict.Current.Knowledge,
		Candidates:  entityExpectations(secondConflict),
	}); err != nil {
		t.Fatal(err)
	}
	continued, err := resolutions.EntityConflict(secondChildContext, secondChildID)
	if err != nil {
		t.Fatal(err)
	}
	var parentCandidate knowledge.Entity
	for _, candidate := range continued.Candidates {
		if candidate.Candidate.Content.Description == "first value" {
			parentCandidate = candidate.Candidate.Content
		}
	}
	if parentCandidate.ID == "" {
		t.Fatalf("continued Child conflict = %#v", continued)
	}
	continuedReview, err := merger.EntityReview(secondChildContext, secondChildID, continued.BaseVersion)
	if err != nil {
		t.Fatal(err)
	}
	if continuedReview.Assignment.ParentVersion != 2 || continuedReview.Parent.Knowledge.Description != "first value" {
		t.Fatalf("continued Child review = %#v", continuedReview)
	}

	if _, err := merger.ResolveEntity(secondChildContext, resolution.EntityCommand{
		Source: provenance.Source{
			ID:         "second-child-resolution-2",
			Kind:       provenance.Resolution,
			ProducerID: "second-child-agent-2",
		},
		BaseVersion: continued.BaseVersion,
		Final:       parentCandidate,
		Candidates:  entityExpectations(continued),
	}); err != nil {
		t.Fatal(err)
	}
	parentFinal, found, err := database.CurrentEntity(parentContext, parentID)
	if err != nil || !found || parentFinal.Version != 2 || parentFinal.Knowledge.Description != "first value" {
		t.Fatalf("final Parent = %#v, found = %v, error = %v", parentFinal, found, err)
	}
	secondChildFinal, found, err := database.CurrentEntity(secondChildContext, secondChildID)
	if err != nil || !found || secondChildFinal.Version != 2 || secondChildFinal.Knowledge.Description != "first value" {
		t.Fatalf("final second Child = %#v, found = %v, error = %v", secondChildFinal, found, err)
	}
	if _, err := resolutions.EntityConflict(secondChildContext, secondChildID); err == nil {
		t.Fatal("second Child conflict remains unresolved")
	}
}

func submitZoneEntity(
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
