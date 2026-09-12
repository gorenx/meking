package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/knowledge/candidate"
)

// Candidates persists and restores unresolved candidate versions. It contains
// no conflict-resolution decisions.
type Candidates struct {
	database *Database
}

func (database *Database) Candidates() *Candidates {
	return &Candidates{
		database: database,
	}
}

func (candidates *Candidates) FindEntity(
	ctx context.Context,
	key candidate.Key[knowledge.EntityID],
) (candidate.Entity, bool, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return candidate.Entity{}, false, err
	}
	row, err := queries.GetEntityCandidate(ctx, db.GetEntityCandidateParams{
		ZoneID:      queries.zoneID,
		EntityID:    string(key.ID),
		BaseVersion: int64(key.BaseVersion),
		ContentHash: key.ContentHash[:],
	})
	if errors.Is(err, sql.ErrNoRows) {
		return candidate.Entity{}, false, nil
	}
	if err != nil {
		return candidate.Entity{}, false, classifySQLite("read Entity Candidate", err)
	}
	entity, err := candidates.restoreEntity(
		ctx,
		queries,
		key,
		row.CandidateNumber,
		row.Description,
	)
	return entity, err == nil, err
}

func (candidates *Candidates) FindRelation(
	ctx context.Context,
	key candidate.Key[knowledge.RelationID],
) (candidate.Relation, bool, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return candidate.Relation{}, false, err
	}
	row, err := queries.GetRelationCandidate(ctx, db.GetRelationCandidateParams{
		ZoneID:      queries.zoneID,
		RelationID:  string(key.ID),
		BaseVersion: int64(key.BaseVersion),
		ContentHash: key.ContentHash[:],
	})
	if errors.Is(err, sql.ErrNoRows) {
		return candidate.Relation{}, false, nil
	}
	if err != nil {
		return candidate.Relation{}, false, classifySQLite("read Relation Candidate", err)
	}
	relation, err := candidates.restoreRelation(
		ctx,
		key,
		row.CandidateNumber,
		row.Description,
	)
	return relation, err == nil, err
}

func (candidates *Candidates) FindClaim(
	ctx context.Context,
	key candidate.Key[knowledge.ClaimID],
) (candidate.Claim, bool, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return candidate.Claim{}, false, err
	}
	row, err := queries.GetClaimCandidate(ctx, db.GetClaimCandidateParams{
		ZoneID:      queries.zoneID,
		ClaimID:     string(key.ID),
		BaseVersion: int64(key.BaseVersion),
		ContentHash: key.ContentHash[:],
	})
	if errors.Is(err, sql.ErrNoRows) {
		return candidate.Claim{}, false, nil
	}
	if err != nil {
		return candidate.Claim{}, false, classifySQLite("read Claim Candidate", err)
	}
	claim, err := candidates.restoreClaim(
		ctx,
		key,
		row.CandidateNumber,
		row.Description,
	)
	return claim, err == nil, err
}

func (candidates *Candidates) NextEntityNumber(
	ctx context.Context,
	entityID knowledge.EntityID,
	baseVersion knowledge.Version,
) (uint64, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return 0, err
	}
	number, err := queries.NextEntityCandidateNumber(ctx, db.NextEntityCandidateNumberParams{
		ZoneID:      queries.zoneID,
		EntityID:    string(entityID),
		BaseVersion: int64(baseVersion),
	})
	return candidateNumber("Entity", number, err)
}

func (candidates *Candidates) NextRelationNumber(
	ctx context.Context,
	relationID knowledge.RelationID,
	baseVersion knowledge.Version,
) (uint64, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return 0, err
	}
	number, err := queries.NextRelationCandidateNumber(ctx, db.NextRelationCandidateNumberParams{
		ZoneID:      queries.zoneID,
		RelationID:  string(relationID),
		BaseVersion: int64(baseVersion),
	})
	return candidateNumber("Relation", number, err)
}

func (candidates *Candidates) NextClaimNumber(
	ctx context.Context,
	claimID knowledge.ClaimID,
	baseVersion knowledge.Version,
) (uint64, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return 0, err
	}
	number, err := queries.NextClaimCandidateNumber(ctx, db.NextClaimCandidateNumberParams{
		ZoneID:      queries.zoneID,
		ClaimID:     string(claimID),
		BaseVersion: int64(baseVersion),
	})
	return candidateNumber("Claim", number, err)
}

func (candidates *Candidates) AddEntity(
	ctx context.Context,
	entity candidate.Entity,
	createdBySourceID string,
) error {
	if err := candidate.ValidateEntity(entity); err != nil {
		return err
	}
	queries, err := candidates.database.writer(ctx)
	if err != nil {
		return err
	}
	if err := queries.CreateEntityCandidate(ctx, db.CreateEntityCandidateParams{
		ZoneID:            queries.zoneID,
		EntityID:          string(entity.Key.ID),
		BaseVersion:       int64(entity.Key.BaseVersion),
		ContentHash:       entity.Key.ContentHash[:],
		CandidateNumber:   int64(entity.Number),
		Description:       entity.Content.Description,
		CreatedBySourceID: createdBySourceID,
	}); err != nil {
		return classifySQLite("add Entity Candidate", err)
	}
	for _, alias := range entity.Content.Aliases {
		if err := queries.CreateEntityCandidateAlias(ctx, db.CreateEntityCandidateAliasParams{
			ZoneID:          queries.zoneID,
			EntityID:        string(entity.Key.ID),
			BaseVersion:     int64(entity.Key.BaseVersion),
			CandidateNumber: int64(entity.Number),
			Alias:           alias,
		}); err != nil {
			return classifySQLite("add Entity Candidate alias", err)
		}
	}
	return nil
}

func (candidates *Candidates) AddRelation(
	ctx context.Context,
	relation candidate.Relation,
	createdBySourceID string,
) error {
	if err := candidate.ValidateRelation(relation); err != nil {
		return err
	}
	queries, err := candidates.database.writer(ctx)
	if err != nil {
		return err
	}
	err = queries.CreateRelationCandidate(ctx, db.CreateRelationCandidateParams{
		ZoneID:            queries.zoneID,
		RelationID:        string(relation.Key.ID),
		BaseVersion:       int64(relation.Key.BaseVersion),
		ContentHash:       relation.Key.ContentHash[:],
		CandidateNumber:   int64(relation.Number),
		Description:       relation.Content.Description,
		CreatedBySourceID: createdBySourceID,
	})
	return classifySQLite("add Relation Candidate", err)
}

func (candidates *Candidates) AddClaim(
	ctx context.Context,
	claim candidate.Claim,
	createdBySourceID string,
) error {
	if err := candidate.ValidateClaim(claim); err != nil {
		return err
	}
	queries, err := candidates.database.writer(ctx)
	if err != nil {
		return err
	}
	err = queries.CreateClaimCandidate(ctx, db.CreateClaimCandidateParams{
		ZoneID:            queries.zoneID,
		ClaimID:           string(claim.Key.ID),
		BaseVersion:       int64(claim.Key.BaseVersion),
		ContentHash:       claim.Key.ContentHash[:],
		CandidateNumber:   int64(claim.Number),
		Description:       claim.Content.Description,
		CreatedBySourceID: createdBySourceID,
	})
	return classifySQLite("add Claim Candidate", err)
}

func (candidates *Candidates) ListEntities(
	ctx context.Context,
	entityID knowledge.EntityID,
	baseVersion knowledge.Version,
) ([]candidate.Entity, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntityCandidates(ctx, db.ListEntityCandidatesParams{
		ZoneID:      queries.zoneID,
		EntityID:    string(entityID),
		BaseVersion: int64(baseVersion),
	})
	if err != nil {
		return nil, classifySQLite("list Entity Candidates", err)
	}
	result := make([]candidate.Entity, 0, len(rows))
	for _, row := range rows {
		hash, err := storedHash(row.ContentHash)
		if err != nil {
			return nil, err
		}
		entity, err := candidates.restoreEntity(
			ctx,
			queries,
			candidate.Key[knowledge.EntityID]{
				ID:          entityID,
				BaseVersion: baseVersion,
				ContentHash: hash,
			},
			row.CandidateNumber,
			row.Description,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, entity)
	}
	return result, nil
}

func (candidates *Candidates) ListRelations(
	ctx context.Context,
	relationID knowledge.RelationID,
	baseVersion knowledge.Version,
) ([]candidate.Relation, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListRelationCandidates(ctx, db.ListRelationCandidatesParams{
		ZoneID:      queries.zoneID,
		RelationID:  string(relationID),
		BaseVersion: int64(baseVersion),
	})
	if err != nil {
		return nil, classifySQLite("list Relation Candidates", err)
	}
	result := make([]candidate.Relation, 0, len(rows))
	for _, row := range rows {
		hash, err := storedHash(row.ContentHash)
		if err != nil {
			return nil, err
		}
		relation, err := candidates.restoreRelation(
			ctx,
			candidate.Key[knowledge.RelationID]{
				ID:          relationID,
				BaseVersion: baseVersion,
				ContentHash: hash,
			},
			row.CandidateNumber,
			row.Description,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, relation)
	}
	return result, nil
}

func (candidates *Candidates) ListClaims(
	ctx context.Context,
	claimID knowledge.ClaimID,
	baseVersion knowledge.Version,
) ([]candidate.Claim, error) {
	queries, err := candidates.database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListClaimCandidates(ctx, db.ListClaimCandidatesParams{
		ZoneID:      queries.zoneID,
		ClaimID:     string(claimID),
		BaseVersion: int64(baseVersion),
	})
	if err != nil {
		return nil, classifySQLite("list Claim Candidates", err)
	}
	result := make([]candidate.Claim, 0, len(rows))
	for _, row := range rows {
		hash, err := storedHash(row.ContentHash)
		if err != nil {
			return nil, err
		}
		claim, err := candidates.restoreClaim(
			ctx,
			candidate.Key[knowledge.ClaimID]{
				ID:          claimID,
				BaseVersion: baseVersion,
				ContentHash: hash,
			},
			row.CandidateNumber,
			row.Description,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, claim)
	}
	return result, nil
}

func (candidates *Candidates) restoreEntity(
	ctx context.Context,
	queries statements,
	key candidate.Key[knowledge.EntityID],
	number int64,
	description string,
) (candidate.Entity, error) {
	identity, found, err := candidates.database.EntityIdentity(ctx, key.ID)
	if err != nil {
		return candidate.Entity{}, err
	}
	if !found {
		return candidate.Entity{}, fmt.Errorf(
			"%w: Entity Candidate identity is missing",
			knowledge.ErrDataIntegrity,
		)
	}
	aliases, err := queries.ListEntityCandidateAliases(ctx, db.ListEntityCandidateAliasesParams{
		ZoneID:          queries.zoneID,
		EntityID:        string(key.ID),
		BaseVersion:     int64(key.BaseVersion),
		CandidateNumber: number,
	})
	if err != nil {
		return candidate.Entity{}, classifySQLite("read Entity Candidate aliases", err)
	}
	entity := candidate.Entity{
		Key:    key,
		Number: uint64(number),
		Content: knowledge.Entity{
			ID:          key.ID,
			Title:       identity.Title,
			Type:        identity.Type,
			Aliases:     aliases,
			Description: description,
		},
	}
	if err := candidate.ValidateEntity(entity); err != nil {
		return candidate.Entity{}, fmt.Errorf(
			"%w: stored Entity Candidate: %v",
			knowledge.ErrDataIntegrity,
			err,
		)
	}
	return entity, nil
}

func (candidates *Candidates) restoreRelation(
	ctx context.Context,
	key candidate.Key[knowledge.RelationID],
	number int64,
	description string,
) (candidate.Relation, error) {
	identity, found, err := candidates.database.RelationKey(ctx, key.ID)
	if err != nil {
		return candidate.Relation{}, err
	}
	if !found {
		return candidate.Relation{}, fmt.Errorf(
			"%w: Relation Candidate identity is missing",
			knowledge.ErrDataIntegrity,
		)
	}
	relation := candidate.Relation{
		Key:    key,
		Number: uint64(number),
		Content: knowledge.Relation{
			ID:             key.ID,
			SourceEntityID: identity.SourceEntityID,
			TargetEntityID: identity.TargetEntityID,
			Type:           identity.Type,
			Description:    description,
		},
	}
	if err := candidate.ValidateRelation(relation); err != nil {
		return candidate.Relation{}, fmt.Errorf(
			"%w: stored Relation Candidate: %v",
			knowledge.ErrDataIntegrity,
			err,
		)
	}
	return relation, nil
}

func (candidates *Candidates) restoreClaim(
	ctx context.Context,
	key candidate.Key[knowledge.ClaimID],
	number int64,
	description string,
) (candidate.Claim, error) {
	identity, found, err := candidates.database.ClaimIdentity(ctx, key.ID)
	if err != nil {
		return candidate.Claim{}, err
	}
	if !found {
		return candidate.Claim{}, fmt.Errorf(
			"%w: Claim Candidate identity is missing",
			knowledge.ErrDataIntegrity,
		)
	}
	claim := candidate.Claim{
		Key:    key,
		Number: uint64(number),
		Content: knowledge.Claim{
			ID:          key.ID,
			Subject:     identity.Subject,
			Type:        identity.Type,
			Description: description,
		},
	}
	if err := candidate.ValidateClaim(claim); err != nil {
		return candidate.Claim{}, fmt.Errorf(
			"%w: stored Claim Candidate: %v",
			knowledge.ErrDataIntegrity,
			err,
		)
	}
	return claim, nil
}

func candidateNumber(kind string, number int64, err error) (uint64, error) {
	if err != nil {
		return 0, classifySQLite("allocate "+kind+" Candidate number", err)
	}
	if number <= 0 {
		return 0, fmt.Errorf(
			"%w: %s Candidate number is %d",
			knowledge.ErrDataIntegrity,
			kind,
			number,
		)
	}
	return uint64(number), nil
}
