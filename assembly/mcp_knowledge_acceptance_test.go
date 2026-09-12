package assembly_test

import (
	"fmt"
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/memoria-space/meking/knowledge"
	mekingmcp "github.com/memoria-space/meking/mcp"
	"github.com/memoria-space/meking/memory"
)

func TestMCPRelationClaimConflictAndDeletionLifecycle(t *testing.T) {
	fixture := openMCPFixture(t, protocol.LATEST_PROTOCOL_VERSION)
	sessionID := string(fixture.sessionID)
	alice := knowledge.EntityIdentity{Title: "ALICE", Type: "PERSON"}
	bob := knowledge.EntityIdentity{Title: "BOB", Type: "PERSON"}
	worksWith := knowledge.RelationIdentity{Source: alice, Target: bob, Type: "WORKS_WITH"}

	initial := mekingmcp.AddMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: sessionID,
		Memory: mekingmcp.MemoryRequest{
			ID: "memory-1",
			Messages: []mekingmcp.MessageRequest{{
				ID: "message-1", Role: "user", Text: "Alice works with Bob and mentors him.",
			}},
			Knowledge: mekingmcp.KnowledgeRequest{
				Entities: []mekingmcp.EntityMemory{
					{
						Content: knowledge.EntityContent{Identity: alice, Aliases: []string{"Alice"}, Description: "Alice is an engineer."},
						Source:  mekingmcp.EntitySource{Frequency: 1},
					},
					{
						Content: knowledge.EntityContent{Identity: bob, Aliases: []string{"Bob"}, Description: "Bob is a designer."},
						Source:  mekingmcp.EntitySource{Frequency: 1},
					},
				},
				Relations: []mekingmcp.RelationMemory{
					{
						Content: knowledge.RelationContent{Source: alice, Target: bob, Type: "WORKS_WITH", Description: "Alice works with Bob."},
						Source:  mekingmcp.RelationSource{Weight: 1},
					},
					{
						Content: knowledge.RelationContent{Source: alice, Target: bob, Type: "MENTORS", Description: "Alice mentors Bob."},
						Source:  mekingmcp.RelationSource{Weight: 1},
					},
				},
				Claims: []mekingmcp.ClaimMemory{
					{
						Subject:     mekingmcp.ClaimSubject{Entity: &alice},
						Type:        "ROLE",
						Description: "Alice is an engineer.",
						Source:      mekingmcp.ClaimSource{SubjectText: "Alice", ObjectText: "engineer", SourceText: "Alice is an engineer."},
					},
					{
						Subject:     mekingmcp.ClaimSubject{Relation: &worksWith},
						Type:        "SINCE",
						Description: "Alice has worked with Bob since 2020.",
						Source:      mekingmcp.ClaimSource{SubjectText: "Alice and Bob", ObjectText: "2020", StartDate: "2020", SourceText: "Alice works with Bob."},
					},
				},
			},
		},
	}
	added := callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", initial)
	if len(added.Conflicts.Entities) != 0 || len(added.Conflicts.Relations) != 0 ||
		len(added.Conflicts.Claims) != 0 {
		t.Fatalf("initial conflicts = %#v", added.Conflicts)
	}

	current := callTool[mekingmcp.MemoryMatches](t, fixture.client, "search_memory", mekingmcp.SearchMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: sessionID,
		Query:     memory.Query{Title: alice.Title, Type: alice.Type},
	})
	if len(current.Entities) != 1 || len(current.Relations) != 2 || len(current.Claims) != 2 {
		t.Fatalf("initial Knowledge = %#v", current)
	}
	aliceID := current.Entities[0].Version.Content.ID
	workRelation := relationMatchByType(t, current.Relations, "WORKS_WITH")
	mentorRelation := relationMatchByType(t, current.Relations, "MENTORS")
	roleClaim := claimMatchByType(t, current.Claims, "ROLE")
	sinceClaim := claimMatchByType(t, current.Claims, "SINCE")
	readResource[mekingmcp.RelationResource](
		t,
		fixture.client,
		fmt.Sprintf("meking://users/%s/sessions/%s/knowledge/relations/%s", mcpTestUserID, sessionID, workRelation.Version.Content.ID),
	)
	readResource[mekingmcp.ClaimResource](
		t,
		fixture.client,
		fmt.Sprintf("meking://users/%s/sessions/%s/knowledge/claims/%s", mcpTestUserID, sessionID, roleClaim.Version.Content.ID),
	)

	changed := mekingmcp.AddMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: sessionID,
		Memory: mekingmcp.MemoryRequest{
			ID:       "memory-2",
			Messages: []mekingmcp.MessageRequest{{ID: "message-2", Role: "user", Text: "Alice collaborates with Bob and now works as a designer."}},
			Knowledge: mekingmcp.KnowledgeRequest{
				Entities: []mekingmcp.EntityMemory{},
				Relations: []mekingmcp.RelationMemory{{
					Content: knowledge.RelationContent{Source: alice, Target: bob, Type: "WORKS_WITH", Description: "Alice collaborates with Bob."},
					Source:  mekingmcp.RelationSource{Weight: 2},
				}},
				Claims: []mekingmcp.ClaimMemory{{
					Subject:     mekingmcp.ClaimSubject{Entity: &alice},
					Type:        "ROLE",
					Description: "Alice is a designer.",
					Source:      mekingmcp.ClaimSource{SubjectText: "Alice", ObjectText: "designer", SourceText: "Alice now works as a designer."},
				}},
			},
		},
	}
	conflicting := callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", changed)
	if len(conflicting.Conflicts.Relations) != 1 || len(conflicting.Conflicts.Claims) != 1 {
		t.Fatalf("changed conflicts = %#v", conflicting.Conflicts)
	}
	relationConflict := conflicting.Conflicts.Relations[0]
	claimConflict := conflicting.Conflicts.Claims[0]
	if relationConflict.Current == nil || len(relationConflict.Evidence) != 1 ||
		len(relationConflict.Alternatives) != 1 || len(relationConflict.Alternatives[0].Evidence) != 1 ||
		claimConflict.Current == nil || len(claimConflict.Alternatives) != 1 {
		t.Fatalf("incomplete conflicts = relation %#v, claim %#v", relationConflict, claimConflict)
	}
	formalDuringConflict := callTool[mekingmcp.MemoryMatches](t, fixture.client, "search_memory", mekingmcp.SearchMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: sessionID,
		Query:     memory.Query{Title: alice.Title, Type: alice.Type},
	})
	formalRelation := relationMatchByType(t, formalDuringConflict.Relations, "WORKS_WITH")
	formalClaim := claimMatchByType(t, formalDuringConflict.Claims, "ROLE")
	if formalRelation.Version.Number != 1 || formalRelation.Version.Content.Description != "Alice works with Bob." ||
		formalClaim.Version.Number != 1 || formalClaim.Version.Content.Description != "Alice is an engineer." {
		t.Fatalf("search exposed Candidate content = %#v", formalDuringConflict)
	}

	listedRelations := callTool[mekingmcp.Page[mekingmcp.RelationConflict]](
		t,
		fixture.client,
		"list_relation_conflicts",
		mekingmcp.ConflictPageRequest{UserID: mcpTestUserID, SessionID: sessionID},
	)
	listedClaims := callTool[mekingmcp.Page[mekingmcp.ClaimConflict]](
		t,
		fixture.client,
		"list_claim_conflicts",
		mekingmcp.ConflictPageRequest{UserID: mcpTestUserID, SessionID: sessionID},
	)
	if len(listedRelations.Items) != 1 || len(listedClaims.Items) != 1 {
		t.Fatalf("conflict pages = relation %#v, claim %#v", listedRelations, listedClaims)
	}

	relationConflict = callTool[mekingmcp.RelationConflict](
		t,
		fixture.client,
		"get_relation_conflict",
		mekingmcp.RelationConflictRequest{UserID: mcpTestUserID, SessionID: sessionID, RelationID: relationConflict.ID},
	)
	claimConflict = callTool[mekingmcp.ClaimConflict](
		t,
		fixture.client,
		"get_claim_conflict",
		mekingmcp.ClaimConflictRequest{UserID: mcpTestUserID, SessionID: sessionID, ClaimID: claimConflict.ID},
	)
	relationResolution := callTool[mekingmcp.VersionChange](
		t,
		fixture.client,
		"resolve_relation_conflict",
		mekingmcp.RelationDecision{
			UserID:    mcpTestUserID,
			SessionID: sessionID,
			SourceID:  "resolution-relation-1",
			Choice: mekingmcp.RelationChoice{
				BaseVersion:  relationConflict.BaseVersion,
				Final:        relationConflict.Current.Content,
				Alternatives: conflictReferences(relationConflict.Alternatives),
			},
		},
	)
	if relationResolution.Version != 1 || relationResolution.Created {
		t.Fatalf("relation resolution = %#v", relationResolution)
	}
	claimResolution := callTool[mekingmcp.VersionChange](
		t,
		fixture.client,
		"resolve_claim_conflict",
		mekingmcp.ClaimDecision{
			UserID:    mcpTestUserID,
			SessionID: sessionID,
			SourceID:  "resolution-claim-1",
			Choice: mekingmcp.ClaimChoice{
				BaseVersion:  claimConflict.BaseVersion,
				Final:        claimConflict.Alternatives[0].Content,
				Alternatives: conflictReferences(claimConflict.Alternatives),
			},
		},
	)
	if claimResolution.Version != 2 || !claimResolution.Created {
		t.Fatalf("claim resolution = %#v", claimResolution)
	}

	reopenedRequest := changed
	reopenedRequest.Memory.ID = "memory-3"
	reopenedRequest.Memory.Messages[0].ID = "message-3"
	reopenedRequest.Memory.Knowledge.Claims = []mekingmcp.ClaimMemory{}
	reopened := callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", reopenedRequest)
	if len(reopened.Conflicts.Relations) != 1 ||
		reopened.Conflicts.Relations[0].Alternatives[0].Revision <= relationConflict.Alternatives[0].Revision {
		t.Fatalf("reopened Relation conflict = %#v", reopened.Conflicts.Relations)
	}
	stale := reopened.Conflicts.Relations[0]

	newAlternative := reopenedRequest
	newAlternative.Memory.ID = "memory-4"
	newAlternative.Memory.Messages[0].ID = "message-4"
	newAlternative.Memory.Knowledge.Relations[0].Content.Description = "Alice partners with Bob."
	callTool[mekingmcp.MemoryReceipt](t, fixture.client, "add_memory", newAlternative)
	assertToolFailure(
		t,
		rawToolCall(t, fixture.client, "resolve_relation_conflict", mekingmcp.RelationDecision{
			UserID:    mcpTestUserID,
			SessionID: sessionID,
			SourceID:  "resolution-relation-stale",
			Choice: mekingmcp.RelationChoice{
				BaseVersion:  stale.BaseVersion,
				Final:        stale.Current.Content,
				Alternatives: conflictReferences(stale.Alternatives),
			},
		}),
		"conflict_changed",
	)
	latestRelationConflict := callTool[mekingmcp.RelationConflict](
		t,
		fixture.client,
		"get_relation_conflict",
		mekingmcp.RelationConflictRequest{UserID: mcpTestUserID, SessionID: sessionID, RelationID: stale.ID},
	)
	callTool[mekingmcp.VersionChange](
		t,
		fixture.client,
		"resolve_relation_conflict",
		mekingmcp.RelationDecision{
			UserID:    mcpTestUserID,
			SessionID: sessionID,
			SourceID:  "resolution-relation-2",
			Choice: mekingmcp.RelationChoice{
				BaseVersion:  latestRelationConflict.BaseVersion,
				Final:        latestRelationConflict.Current.Content,
				Alternatives: conflictReferences(latestRelationConflict.Alternatives),
			},
		},
	)
	resolvedView := callTool[mekingmcp.MemoryMatches](t, fixture.client, "search_memory", mekingmcp.SearchMemoryRequest{
		UserID:    mcpTestUserID,
		SessionID: sessionID,
		Query:     memory.Query{Title: alice.Title, Type: alice.Type},
	})
	resolvedRelation := relationMatchByType(t, resolvedView.Relations, "WORKS_WITH")
	resolvedClaim := claimMatchByType(t, resolvedView.Claims, "ROLE")
	if resolvedRelation.Version.Number != 1 || resolvedRelation.Version.Content.Description != "Alice works with Bob." ||
		resolvedClaim.Version.Number != 2 || resolvedClaim.Version.Content.Description != "Alice is a designer." {
		t.Fatalf("resolved formal Knowledge = %#v", resolvedView)
	}

	assertToolFailure(
		t,
		rawToolCall(t, fixture.client, "delete_entity", mekingmcp.DeleteEntityRequest{
			UserID:    mcpTestUserID,
			SessionID: sessionID,
			SourceID:  "delete-entity-blocked",
			Target:    knowledge.Reference[knowledge.EntityID]{ID: aliceID, Version: 1},
		}),
		"knowledge_in_use",
	)
	deleteClaim(t, fixture, "delete-role", roleClaim.Version.Content.ID, 2, 3)
	deleteClaim(t, fixture, "delete-since", sinceClaim.Version.Content.ID, 1, 2)
	deleteRelation(t, fixture, "delete-work", workRelation.Version.Content.ID, 1, 2)
	deleteRelation(t, fixture, "delete-mentor", mentorRelation.Version.Content.ID, 1, 2)
	deletedEntity := callTool[mekingmcp.VersionChange](
		t,
		fixture.client,
		"delete_entity",
		mekingmcp.DeleteEntityRequest{
			UserID:    mcpTestUserID,
			SessionID: sessionID,
			SourceID:  "delete-alice",
			Target:    knowledge.Reference[knowledge.EntityID]{ID: aliceID, Version: 1},
		},
	)
	if deletedEntity.Version != 2 || !deletedEntity.Created {
		t.Fatalf("deleted Entity = %#v", deletedEntity)
	}
	replayedDelete := callTool[mekingmcp.VersionChange](
		t,
		fixture.client,
		"delete_entity",
		mekingmcp.DeleteEntityRequest{
			UserID:    mcpTestUserID,
			SessionID: sessionID,
			SourceID:  "delete-alice",
			Target:    knowledge.Reference[knowledge.EntityID]{ID: aliceID, Version: 1},
		},
	)
	if replayedDelete != deletedEntity {
		t.Fatalf("replayed deletion = %#v, want %#v", replayedDelete, deletedEntity)
	}
	tombstone := readResource[mekingmcp.EntityResource](
		t,
		fixture.client,
		fmt.Sprintf("meking://users/%s/sessions/%s/knowledge/entities/%s", mcpTestUserID, sessionID, aliceID),
	)
	if !tombstone.Version.Deleted {
		t.Fatalf("Entity tombstone = %#v", tombstone)
	}
}

func relationMatchByType(
	t *testing.T,
	matches []mekingmcp.RelationMatch,
	relationType string,
) mekingmcp.RelationMatch {
	t.Helper()
	for _, match := range matches {
		if match.Version.Content.Type == relationType {
			return match
		}
	}
	t.Fatalf("Relation Type %q not found in %#v", relationType, matches)
	return mekingmcp.RelationMatch{}
}

func claimMatchByType(
	t *testing.T,
	matches []mekingmcp.ClaimMatch,
	claimType string,
) mekingmcp.ClaimMatch {
	t.Helper()
	for _, match := range matches {
		if match.Version.Content.Type == claimType {
			return match
		}
	}
	t.Fatalf("Claim Type %q not found in %#v", claimType, matches)
	return mekingmcp.ClaimMatch{}
}

func conflictReferences[T any, E any](alternatives []mekingmcp.Alternative[T, E]) []mekingmcp.ConflictReference {
	references := make([]mekingmcp.ConflictReference, len(alternatives))
	for index, alternative := range alternatives {
		references[index] = alternative.Reference
	}
	return references
}

func deleteClaim(
	t *testing.T,
	fixture mcpFixture,
	sourceID string,
	claimID string,
	expectedVersion uint64,
	deletedVersion uint64,
) {
	t.Helper()
	result := callTool[mekingmcp.VersionChange](
		t,
		fixture.client,
		"delete_claim",
		mekingmcp.DeleteClaimRequest{
			UserID:    mcpTestUserID,
			SessionID: string(fixture.sessionID),
			SourceID:  sourceID,
			Target: knowledge.Reference[knowledge.ClaimID]{
				ID: knowledge.ClaimID(claimID), Version: knowledge.Version(expectedVersion),
			},
		},
	)
	if result.Version != deletedVersion || !result.Created {
		t.Fatalf("delete Claim result = %#v", result)
	}
}

func deleteRelation(
	t *testing.T,
	fixture mcpFixture,
	sourceID string,
	relationID knowledge.RelationID,
	expectedVersion uint64,
	deletedVersion uint64,
) {
	t.Helper()
	result := callTool[mekingmcp.VersionChange](
		t,
		fixture.client,
		"delete_relation",
		mekingmcp.DeleteRelationRequest{
			UserID:    mcpTestUserID,
			SessionID: string(fixture.sessionID),
			SourceID:  sourceID,
			Target: knowledge.Reference[knowledge.RelationID]{
				ID: relationID, Version: knowledge.Version(expectedVersion),
			},
		},
	)
	if result.Version != deletedVersion || !result.Created {
		t.Fatalf("delete Relation result = %#v", result)
	}
}
