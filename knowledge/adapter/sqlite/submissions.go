package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/knowledge/candidate"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
)

func (database *Database) addCandidateResults(
	ctx context.Context,
	queries statements,
	sourceID string,
	result *submission.Result,
) error {
	entities, err := queries.ListSourceEntityCandidates(ctx, db.ListSourceEntityCandidatesParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return classifySQLite("read Entity Candidate results", err)
	}
	for _, row := range entities {
		hash, err := storedHash(row.ContentHash)
		if err != nil {
			return err
		}
		key := candidate.Key[knowledge.EntityID]{
			ID:          knowledge.EntityID(row.EntityID),
			BaseVersion: knowledge.Version(row.BaseVersion),
			ContentHash: hash,
		}
		if row.OpenedCandidate {
			result.OpenedCandidates.Entities = append(result.OpenedCandidates.Entities, key)
		} else {
			result.ReusedCandidates.Entities = append(result.ReusedCandidates.Entities, key)
		}
	}
	relations, err := queries.ListSourceRelationCandidates(ctx, db.ListSourceRelationCandidatesParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return classifySQLite("read Relation Candidate results", err)
	}
	for _, row := range relations {
		hash, err := storedHash(row.ContentHash)
		if err != nil {
			return err
		}
		key := candidate.Key[knowledge.RelationID]{
			ID:          knowledge.RelationID(row.RelationID),
			BaseVersion: knowledge.Version(row.BaseVersion),
			ContentHash: hash,
		}
		if row.OpenedCandidate {
			result.OpenedCandidates.Relations = append(result.OpenedCandidates.Relations, key)
		} else {
			result.ReusedCandidates.Relations = append(result.ReusedCandidates.Relations, key)
		}
	}
	claims, err := queries.ListSourceClaimCandidates(ctx, db.ListSourceClaimCandidatesParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return classifySQLite("read Claim Candidate results", err)
	}
	for _, row := range claims {
		hash, err := storedHash(row.ContentHash)
		if err != nil {
			return err
		}
		key := candidate.Key[knowledge.ClaimID]{
			ID:          knowledge.ClaimID(row.ClaimID),
			BaseVersion: knowledge.Version(row.BaseVersion),
			ContentHash: hash,
		}
		if row.OpenedCandidate {
			result.OpenedCandidates.Claims = append(result.OpenedCandidates.Claims, key)
		} else {
			result.ReusedCandidates.Claims = append(result.ReusedCandidates.Claims, key)
		}
	}
	return nil
}

func (database *Database) SubmitEntity(ctx context.Context, value provenance.EntitySubmission) error {
	if err := validateEntitySubmission(value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	candidate, err := queries.GetEntityCandidate(ctx, db.GetEntityCandidateParams{ZoneID: queries.zoneID,
		EntityID: string(value.Candidate.ID), BaseVersion: int64(value.Candidate.BaseVersion), ContentHash: value.Candidate.ContentHash[:]})
	if err != nil {
		return classifySQLite("read Entity Candidate", err)
	}
	metadata, err := encodeEntityMetadata(value.Metadata.Frequency)
	if err != nil {
		return err
	}
	return database.saveSourceSupport(ctx, sourceSupport{SourceID: value.SourceID, KnowledgeKind: "entity",
		KnowledgeID: string(value.Candidate.ID), Version: value.Candidate.BaseVersion,
		CandidateNumber: uint64(candidate.CandidateNumber), MetadataJSON: metadata, Evidence: value.Metadata.Evidence})
}

func (database *Database) SubmitRelation(ctx context.Context, value provenance.RelationSubmission) error {
	if err := validateRelationSubmission(value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	candidate, err := queries.GetRelationCandidate(ctx, db.GetRelationCandidateParams{ZoneID: queries.zoneID,
		RelationID: string(value.Candidate.ID), BaseVersion: int64(value.Candidate.BaseVersion), ContentHash: value.Candidate.ContentHash[:]})
	if err != nil {
		return classifySQLite("read Relation Candidate", err)
	}
	metadata, err := encodeRelationMetadata(value.Metadata.Weight)
	if err != nil {
		return err
	}
	return database.saveSourceSupport(ctx, sourceSupport{SourceID: value.SourceID, KnowledgeKind: "relation",
		KnowledgeID: string(value.Candidate.ID), Version: value.Candidate.BaseVersion,
		CandidateNumber: uint64(candidate.CandidateNumber), MetadataJSON: metadata, Evidence: value.Metadata.Evidence})
}

func (database *Database) SubmitClaim(ctx context.Context, value provenance.ClaimSubmission) error {
	if err := validateClaimSubmission(value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	candidate, err := queries.GetClaimCandidate(ctx, db.GetClaimCandidateParams{ZoneID: queries.zoneID,
		ClaimID: string(value.Candidate.ID), BaseVersion: int64(value.Candidate.BaseVersion), ContentHash: value.Candidate.ContentHash[:]})
	if err != nil {
		return classifySQLite("read Claim Candidate", err)
	}
	metadata, err := encodeClaimMetadata(value.Metadata)
	if err != nil {
		return err
	}
	return database.saveSourceSupport(ctx, sourceSupport{SourceID: value.SourceID, KnowledgeKind: "claim",
		KnowledgeID: string(value.Candidate.ID), Version: value.Candidate.BaseVersion,
		CandidateNumber: uint64(candidate.CandidateNumber), MetadataJSON: metadata, Evidence: claimEvidence(value.Metadata)})
}

func (database *Database) EntitySubmissions(ctx context.Context, key candidate.Key[knowledge.EntityID]) ([]provenance.EntitySubmission, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntityCandidateSources(ctx, db.ListEntityCandidateSourcesParams{ZoneID: queries.zoneID,
		EntityID: string(key.ID), BaseVersion: int64(key.BaseVersion), ContentHash: key.ContentHash[:]})
	if err != nil {
		return nil, classifySQLite("list Entity Candidate Sources", err)
	}
	result := make([]provenance.EntitySubmission, 0, len(rows))
	for _, row := range rows {
		metadata, err := decodeEntityMetadata(row.MetadataJson)
		if err != nil {
			return nil, err
		}
		evidence, err := database.sourceEvidence(ctx, queries, row.SourceID)
		if err != nil {
			return nil, err
		}
		result = append(result, provenance.EntitySubmission{Candidate: key, SourceID: row.SourceID,
			OpenedCandidate: row.OpenedCandidate,
			Metadata:        provenance.EntityMetadata{Frequency: metadata.Frequency, Evidence: evidence}})
	}
	return result, nil
}

func (database *Database) RelationSubmissions(ctx context.Context, key candidate.Key[knowledge.RelationID]) ([]provenance.RelationSubmission, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListRelationCandidateSources(ctx, db.ListRelationCandidateSourcesParams{ZoneID: queries.zoneID,
		RelationID: string(key.ID), BaseVersion: int64(key.BaseVersion), ContentHash: key.ContentHash[:]})
	if err != nil {
		return nil, classifySQLite("list Relation Candidate Sources", err)
	}
	result := make([]provenance.RelationSubmission, 0, len(rows))
	for _, row := range rows {
		metadata, err := decodeRelationMetadata(row.MetadataJson)
		if err != nil {
			return nil, err
		}
		evidence, err := database.sourceEvidence(ctx, queries, row.SourceID)
		if err != nil {
			return nil, err
		}
		result = append(result, provenance.RelationSubmission{Candidate: key, SourceID: row.SourceID,
			OpenedCandidate: row.OpenedCandidate,
			Metadata:        provenance.RelationMetadata{Weight: metadata.Weight, Evidence: evidence}})
	}
	return result, nil
}

func (database *Database) ClaimSubmissions(ctx context.Context, key candidate.Key[knowledge.ClaimID]) ([]provenance.ClaimSubmission, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListClaimCandidateSources(ctx, db.ListClaimCandidateSourcesParams{ZoneID: queries.zoneID,
		ClaimID: string(key.ID), BaseVersion: int64(key.BaseVersion), ContentHash: key.ContentHash[:]})
	if err != nil {
		return nil, classifySQLite("list Claim Candidate Sources", err)
	}
	result := make([]provenance.ClaimSubmission, 0, len(rows))
	for _, row := range rows {
		metadata, err := decodeClaimMetadata(row.MetadataJson)
		if err != nil {
			return nil, err
		}
		evidence, err := database.sourceEvidence(ctx, queries, row.SourceID)
		if err != nil {
			return nil, err
		}
		result = append(result, provenance.ClaimSubmission{Candidate: key, SourceID: row.SourceID,
			OpenedCandidate: row.OpenedCandidate,
			Metadata:        claimMetadataValues(metadata.Records, evidence)})
	}
	return result, nil
}

func (database *Database) ResolveEntityCandidate(
	ctx context.Context,
	resolution provenance.EntityCandidateResolution,
) error {
	if err := candidate.ValidateEntityKey(resolution.Candidate); err != nil {
		return err
	}
	if err := validateResolutionSourceID(resolution.SourceID); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	count, err := queries.ResolveEntityCandidate(ctx, db.ResolveEntityCandidateParams{ResolutionSourceID: sql.NullString{String: resolution.SourceID, Valid: true},
		ZoneID: queries.zoneID, EntityID: string(resolution.Candidate.ID), BaseVersion: int64(resolution.Candidate.BaseVersion), ContentHash: resolution.Candidate.ContentHash[:]})
	return resolvedCandidate("Entity", count, err)
}
func (database *Database) ResolveRelationCandidate(
	ctx context.Context,
	resolution provenance.RelationCandidateResolution,
) error {
	if err := candidate.ValidateRelationKey(resolution.Candidate); err != nil {
		return err
	}
	if err := validateResolutionSourceID(resolution.SourceID); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	count, err := queries.ResolveRelationCandidate(ctx, db.ResolveRelationCandidateParams{ResolutionSourceID: sql.NullString{String: resolution.SourceID, Valid: true},
		ZoneID: queries.zoneID, RelationID: string(resolution.Candidate.ID), BaseVersion: int64(resolution.Candidate.BaseVersion), ContentHash: resolution.Candidate.ContentHash[:]})
	return resolvedCandidate("Relation", count, err)
}
func (database *Database) ResolveClaimCandidate(
	ctx context.Context,
	resolution provenance.ClaimCandidateResolution,
) error {
	if err := candidate.ValidateClaimKey(resolution.Candidate); err != nil {
		return err
	}
	if err := validateResolutionSourceID(resolution.SourceID); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	count, err := queries.ResolveClaimCandidate(ctx, db.ResolveClaimCandidateParams{ResolutionSourceID: sql.NullString{String: resolution.SourceID, Valid: true},
		ZoneID: queries.zoneID, ClaimID: string(resolution.Candidate.ID), BaseVersion: int64(resolution.Candidate.BaseVersion), ContentHash: resolution.Candidate.ContentHash[:]})
	return resolvedCandidate("Claim", count, err)
}

func resolvedCandidate(kind string, count int64, err error) error {
	if err != nil {
		return classifySQLite("resolve "+kind+" Candidate", err)
	}
	if count != 1 {
		return fmt.Errorf("%w: %s Candidate is no longer pending", resolution.ErrConflictChanged, kind)
	}
	return nil
}
func validateResolutionSourceID(resolutionSourceID string) error {
	if resolutionSourceID == "" {
		return fmt.Errorf("%w: Resolution Source ID is required", knowledge.ErrInvalidChange)
	}
	return nil
}
func validateEntitySubmission(value provenance.EntitySubmission) error {
	if err := candidate.ValidateEntityKey(value.Candidate); err != nil {
		return err
	}
	if value.SourceID == "" {
		return fmt.Errorf("%w: new Entity Candidate Source is invalid", knowledge.ErrInvalidChange)
	}
	return provenance.ValidateEntityMetadata(value.Metadata)
}
func validateRelationSubmission(value provenance.RelationSubmission) error {
	if err := candidate.ValidateRelationKey(value.Candidate); err != nil {
		return err
	}
	if value.SourceID == "" {
		return fmt.Errorf("%w: new Relation Candidate Source is invalid", knowledge.ErrInvalidChange)
	}
	return provenance.ValidateRelationMetadata(value.Metadata)
}
func validateClaimSubmission(value provenance.ClaimSubmission) error {
	if err := candidate.ValidateClaimKey(value.Candidate); err != nil {
		return err
	}
	if value.SourceID == "" {
		return fmt.Errorf("%w: new Claim Candidate Source is invalid", knowledge.ErrInvalidChange)
	}
	for _, metadata := range value.Metadata {
		if err := provenance.ValidateClaimMetadata(metadata); err != nil {
			return err
		}
	}
	return nil
}
