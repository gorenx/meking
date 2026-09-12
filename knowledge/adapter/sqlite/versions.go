package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
)

func (database *Database) CurrentEntity(
	ctx context.Context,
	id knowledge.EntityID,
) (knowledge.KnowledgeVersion[knowledge.Entity], bool, error) {
	if err := knowledge.ValidateEntityID(id); err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, err
	}
	return readCurrentEntity(ctx, queries, id)
}

func readCurrentEntity(
	ctx context.Context,
	queries statements,
	id knowledge.EntityID,
) (knowledge.KnowledgeVersion[knowledge.Entity], bool, error) {
	row, err := queries.GetCurrentEntity(ctx, db.GetCurrentEntityParams{
		ZoneID:   queries.zoneID,
		EntityID: string(id),
	})
	return restoreEntity(ctx, queries, entityVersionRecord{
		id: id, title: row.Title, entityType: row.EntityType, version: row.Version,
		deleted: row.Deleted, hash: row.ContentHash, description: row.Description,
	}, err)
}

func (database *Database) CurrentRelation(
	ctx context.Context,
	id knowledge.RelationID,
) (knowledge.KnowledgeVersion[knowledge.Relation], bool, error) {
	if err := knowledge.ValidateRelationID(id); err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, err
	}
	return readCurrentRelation(ctx, queries, id)
}

func readCurrentRelation(
	ctx context.Context,
	queries statements,
	id knowledge.RelationID,
) (knowledge.KnowledgeVersion[knowledge.Relation], bool, error) {
	row, err := queries.GetCurrentRelation(ctx, db.GetCurrentRelationParams{
		ZoneID:     queries.zoneID,
		RelationID: string(id),
	})
	return restoreRelation(relationVersionRecord{
		id: id, sourceEntityID: row.SourceEntityID, targetEntityID: row.TargetEntityID,
		relationType: row.RelationType, version: row.Version, deleted: row.Deleted,
		hash: row.ContentHash, description: row.Description,
	}, err)
}

func (database *Database) CurrentClaim(
	ctx context.Context,
	id knowledge.ClaimID,
) (knowledge.KnowledgeVersion[knowledge.Claim], bool, error) {
	if err := knowledge.ValidateClaimID(id); err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, err
	}
	return readCurrentClaim(ctx, queries, id)
}

func readCurrentClaim(
	ctx context.Context,
	queries statements,
	id knowledge.ClaimID,
) (knowledge.KnowledgeVersion[knowledge.Claim], bool, error) {
	row, err := queries.GetCurrentClaim(ctx, db.GetCurrentClaimParams{
		ZoneID:  queries.zoneID,
		ClaimID: string(id),
	})
	return restoreClaim(claimVersionRecord{
		id: id, subjectKind: row.SubjectKind, subjectID: row.SubjectID, claimType: row.ClaimType,
		version: row.Version, deleted: row.Deleted, hash: row.ContentHash, description: row.Description,
	}, err)
}

func (database *Database) AppendEntityVersion(
	ctx context.Context,
	version knowledge.KnowledgeVersion[knowledge.Entity],
	createdBySourceID string,
) error {
	if err := validateEntityVersion(version); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	identity, found, err := database.EntityIdentity(ctx, version.Knowledge.ID)
	if err != nil {
		return err
	}
	if !found || identity.Title != version.Knowledge.Title || identity.Type != version.Knowledge.Type {
		return fmt.Errorf("%w: Entity Version does not match its stable identity", knowledge.ErrIdentityConflict)
	}
	current, hasCurrent, err := database.CurrentEntity(ctx, version.Knowledge.ID)
	if err != nil {
		return err
	}
	if err := validateAppend(version.Knowledge.ID, version.Version, version.Deleted, hasCurrent, current.Version); err != nil {
		return err
	}
	if version.Deleted && (!current.Knowledge.Equal(version.Knowledge) || current.Hash != version.Hash) {
		return fmt.Errorf("%w: Entity Tombstone must preserve Current content", knowledge.ErrInvalidChange)
	}
	if err := queries.CreateEntityVersion(ctx, db.CreateEntityVersionParams{
		ZoneID: queries.zoneID, EntityID: string(version.Knowledge.ID), Version: int64(version.Version),
		Deleted: booleanInteger(version.Deleted), ContentHash: version.Hash[:],
		Description: version.Knowledge.Description, CreatedBySourceID: createdBySourceID,
	}); err != nil {
		return classifySQLite("append Entity Version", err)
	}
	for _, alias := range version.Knowledge.Aliases {
		if err := queries.CreateEntityAlias(ctx, db.CreateEntityAliasParams{
			ZoneID: queries.zoneID, EntityID: string(version.Knowledge.ID), Version: int64(version.Version), Alias: alias,
		}); err != nil {
			return classifySQLite("append Entity alias", err)
		}
	}
	return nil
}

func (database *Database) AppendRelationVersion(
	ctx context.Context,
	version knowledge.KnowledgeVersion[knowledge.Relation],
	createdBySourceID string,
) error {
	if err := validateRelationVersion(version); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	identity, found, err := database.RelationKey(ctx, version.Knowledge.ID)
	if err != nil {
		return err
	}
	if !found || identity.SourceEntityID != version.Knowledge.SourceEntityID ||
		identity.TargetEntityID != version.Knowledge.TargetEntityID || identity.Type != version.Knowledge.Type {
		return fmt.Errorf("%w: Relation Version does not match its stable identity", knowledge.ErrIdentityConflict)
	}
	if !version.Deleted {
		for _, endpoint := range []knowledge.EntityID{identity.SourceEntityID, identity.TargetEntityID} {
			active, err := queries.CountActiveEntity(ctx, db.CountActiveEntityParams{
				ZoneID: queries.zoneID, EntityID: string(endpoint),
			})
			if err != nil {
				return classifySQLite("read active Relation endpoint", err)
			}
			if active != 1 {
				return fmt.Errorf("%w: Relation endpoint Entity %q is not active", knowledge.ErrInvalidChange, endpoint)
			}
		}
	}
	current, hasCurrent, err := database.CurrentRelation(ctx, version.Knowledge.ID)
	if err != nil {
		return err
	}
	if err := validateAppend(version.Knowledge.ID, version.Version, version.Deleted, hasCurrent, current.Version); err != nil {
		return err
	}
	if version.Deleted && (current.Knowledge != version.Knowledge || current.Hash != version.Hash) {
		return fmt.Errorf("%w: Relation Tombstone must preserve Current content", knowledge.ErrInvalidChange)
	}
	err = queries.CreateRelationVersion(ctx, db.CreateRelationVersionParams{
		ZoneID: queries.zoneID, RelationID: string(version.Knowledge.ID), Version: int64(version.Version),
		Deleted: booleanInteger(version.Deleted), ContentHash: version.Hash[:],
		Description: version.Knowledge.Description, CreatedBySourceID: createdBySourceID,
	})
	return classifySQLite("append Relation Version", err)
}

func (database *Database) AppendClaimVersion(
	ctx context.Context,
	version knowledge.KnowledgeVersion[knowledge.Claim],
	createdBySourceID string,
) error {
	if err := validateClaimVersion(version); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	identity, found, err := database.ClaimIdentity(ctx, version.Knowledge.ID)
	if err != nil {
		return err
	}
	if !found || identity.Type != version.Knowledge.Type || !knowledge.SameSubject(identity.Subject, version.Knowledge.Subject) {
		return fmt.Errorf("%w: Claim Version does not match its stable identity", knowledge.ErrIdentityConflict)
	}
	if !version.Deleted {
		active, err := activeClaimSubject(ctx, queries, version.Knowledge.Subject)
		if err != nil {
			return err
		}
		if !active {
			return fmt.Errorf("%w: Claim Subject is not active", knowledge.ErrInvalidChange)
		}
	}
	current, hasCurrent, err := database.CurrentClaim(ctx, version.Knowledge.ID)
	if err != nil {
		return err
	}
	if err := validateAppend(version.Knowledge.ID, version.Version, version.Deleted, hasCurrent, current.Version); err != nil {
		return err
	}
	if version.Deleted && (!current.Knowledge.Equal(version.Knowledge) || current.Hash != version.Hash) {
		return fmt.Errorf("%w: Claim Tombstone must preserve Current content", knowledge.ErrInvalidChange)
	}
	err = queries.CreateClaimVersion(ctx, db.CreateClaimVersionParams{
		ZoneID: queries.zoneID, ClaimID: string(version.Knowledge.ID), Version: int64(version.Version),
		Deleted: booleanInteger(version.Deleted), ContentHash: version.Hash[:],
		Description: version.Knowledge.Description, CreatedBySourceID: createdBySourceID,
	})
	return classifySQLite("append Claim Version", err)
}

func (database *Database) ActiveRelations(
	ctx context.Context,
	entityID knowledge.EntityID,
) ([]knowledge.RelationID, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListActiveRelations(ctx, db.ListActiveRelationsParams{
		ZoneID: queries.zoneID, EntityID: string(entityID),
	})
	if err != nil {
		return nil, classifySQLite("read active Entity Relations", err)
	}
	result := make([]knowledge.RelationID, len(rows))
	for index, id := range rows {
		result[index] = knowledge.RelationID(id)
	}
	return result, nil
}

func (database *Database) ActiveClaims(
	ctx context.Context,
	subject knowledge.Subject,
) ([]knowledge.ClaimID, error) {
	subjectKind, subjectID, err := persistedSubject(subject)
	if err != nil {
		return nil, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListActiveClaims(ctx, db.ListActiveClaimsParams{
		ZoneID: queries.zoneID, SubjectKind: subjectKind, SubjectID: subjectID,
	})
	if err != nil {
		return nil, classifySQLite("read active Subject Claims", err)
	}
	result := make([]knowledge.ClaimID, len(rows))
	for index, id := range rows {
		result[index] = knowledge.ClaimID(id)
	}
	return result, nil
}

func validateAppend(
	object knowledge.ObjectRef,
	next knowledge.Version,
	deleted bool,
	hasCurrent bool,
	current knowledge.Version,
) error {
	if !hasCurrent {
		if next != 1 || deleted {
			return &knowledge.VersionConflict{
				Object:   object,
				Expected: next - 1,
				Actual:   0,
			}
		}
		return nil
	}
	if current+1 != next {
		return &knowledge.VersionConflict{
			Object:   object,
			Expected: next - 1,
			Actual:   current,
		}
	}
	return nil
}

func activeClaimSubject(ctx context.Context, queries statements, subject knowledge.Subject) (bool, error) {
	var count int64
	var err error
	switch subject := subject.(type) {
	case knowledge.EntityID:
		count, err = queries.CountActiveEntity(ctx, db.CountActiveEntityParams{
			ZoneID: queries.zoneID, EntityID: string(subject),
		})
	case knowledge.RelationID:
		count, err = queries.CountActiveRelation(ctx, db.CountActiveRelationParams{
			ZoneID: queries.zoneID, RelationID: string(subject),
		})
	default:
		return false, fmt.Errorf("%w: unsupported Claim Subject %T", knowledge.ErrInvalidChange, subject)
	}
	if err != nil {
		return false, classifySQLite("read active Claim Subject", err)
	}
	return count == 1, nil
}

type entityVersionRecord struct {
	id          knowledge.EntityID
	title       string
	entityType  string
	version     int64
	deleted     int64
	hash        []byte
	description string
}

type relationVersionRecord struct {
	id             knowledge.RelationID
	sourceEntityID string
	targetEntityID string
	relationType   string
	version        int64
	deleted        int64
	hash           []byte
	description    string
}

type claimVersionRecord struct {
	id          knowledge.ClaimID
	subjectKind string
	subjectID   string
	claimType   string
	version     int64
	deleted     int64
	hash        []byte
	description string
}

func restoreEntity(
	ctx context.Context,
	queries statements,
	record entityVersionRecord,
	readErr error,
) (knowledge.KnowledgeVersion[knowledge.Entity], bool, error) {
	if errors.Is(readErr, sql.ErrNoRows) {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, nil
	}
	if readErr != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, classifySQLite("read Entity Version", readErr)
	}
	aliases, err := queries.ListEntityAliases(ctx, db.ListEntityAliasesParams{
		ZoneID: queries.zoneID, EntityID: string(record.id), Version: record.version,
	})
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, classifySQLite("read Entity aliases", err)
	}
	hash, err := storedHash(record.hash)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, err
	}
	deletedValue, err := boolValue(record.deleted)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, err
	}
	version := knowledge.KnowledgeVersion[knowledge.Entity]{
		Knowledge: knowledge.Entity{
			ID: record.id, Title: record.title, Type: record.entityType,
			Aliases: aliases, Description: record.description,
		},
		Version: knowledge.Version(record.version), Deleted: deletedValue, Hash: hash,
	}
	return version, true, validateStoredEntity(version)
}

func restoreRelation(
	record relationVersionRecord,
	readErr error,
) (knowledge.KnowledgeVersion[knowledge.Relation], bool, error) {
	if errors.Is(readErr, sql.ErrNoRows) {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, nil
	}
	if readErr != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, classifySQLite("read Relation Version", readErr)
	}
	hash, err := storedHash(record.hash)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, err
	}
	deletedValue, err := boolValue(record.deleted)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, err
	}
	version := knowledge.KnowledgeVersion[knowledge.Relation]{
		Knowledge: knowledge.Relation{
			ID: record.id, SourceEntityID: knowledge.EntityID(record.sourceEntityID),
			TargetEntityID: knowledge.EntityID(record.targetEntityID),
			Type:           record.relationType, Description: record.description,
		},
		Version: knowledge.Version(record.version), Deleted: deletedValue, Hash: hash,
	}
	return version, true, validateStoredRelation(version)
}

func restoreClaim(
	record claimVersionRecord,
	readErr error,
) (knowledge.KnowledgeVersion[knowledge.Claim], bool, error) {
	if errors.Is(readErr, sql.ErrNoRows) {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, nil
	}
	if readErr != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, classifySQLite("read Claim Version", readErr)
	}
	subject, err := restoredSubject(record.subjectKind, record.subjectID)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, err
	}
	hash, err := storedHash(record.hash)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, err
	}
	deletedValue, err := boolValue(record.deleted)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, err
	}
	version := knowledge.KnowledgeVersion[knowledge.Claim]{
		Knowledge: knowledge.Claim{
			ID: record.id, Subject: subject, Type: record.claimType, Description: record.description,
		},
		Version: knowledge.Version(record.version), Deleted: deletedValue, Hash: hash,
	}
	return version, true, validateStoredClaim(version)
}

func validateEntityVersion(version knowledge.KnowledgeVersion[knowledge.Entity]) error {
	if version.Version == 0 {
		return fmt.Errorf("%w: Entity Version must be positive", knowledge.ErrInvalidChange)
	}
	if err := knowledge.ValidateVersion(version.Version); err != nil {
		return err
	}
	if err := knowledge.ValidateEntity(version.Knowledge); err != nil {
		return err
	}
	hash, err := knowledge.HashEntity(version.Knowledge)
	if err != nil || hash != version.Hash {
		return fmt.Errorf("%w: Entity Version Hash is invalid", knowledge.ErrInvalidChange)
	}
	return nil
}

func validateRelationVersion(version knowledge.KnowledgeVersion[knowledge.Relation]) error {
	if version.Version == 0 {
		return fmt.Errorf("%w: Relation Version must be positive", knowledge.ErrInvalidChange)
	}
	if err := knowledge.ValidateVersion(version.Version); err != nil {
		return err
	}
	if err := knowledge.ValidateRelation(version.Knowledge); err != nil {
		return err
	}
	hash, err := knowledge.HashRelation(version.Knowledge)
	if err != nil || hash != version.Hash {
		return fmt.Errorf("%w: Relation Version Hash is invalid", knowledge.ErrInvalidChange)
	}
	return nil
}

func validateClaimVersion(version knowledge.KnowledgeVersion[knowledge.Claim]) error {
	if version.Version == 0 {
		return fmt.Errorf("%w: Claim Version must be positive", knowledge.ErrInvalidChange)
	}
	if err := knowledge.ValidateVersion(version.Version); err != nil {
		return err
	}
	if err := knowledge.ValidateClaim(version.Knowledge); err != nil {
		return err
	}
	hash, err := knowledge.HashClaim(version.Knowledge)
	if err != nil || hash != version.Hash {
		return fmt.Errorf("%w: Claim Version Hash is invalid", knowledge.ErrInvalidChange)
	}
	return nil
}

func validateStoredEntity(record knowledge.KnowledgeVersion[knowledge.Entity]) error {
	if err := validateEntityVersion(record); err != nil {
		return fmt.Errorf("%w: stored Entity Version: %v", knowledge.ErrDataIntegrity, err)
	}
	return nil
}

func validateStoredRelation(record knowledge.KnowledgeVersion[knowledge.Relation]) error {
	if err := validateRelationVersion(record); err != nil {
		return fmt.Errorf("%w: stored Relation Version: %v", knowledge.ErrDataIntegrity, err)
	}
	return nil
}

func validateStoredClaim(record knowledge.KnowledgeVersion[knowledge.Claim]) error {
	if err := validateClaimVersion(record); err != nil {
		return fmt.Errorf("%w: stored Claim Version: %v", knowledge.ErrDataIntegrity, err)
	}
	return nil
}

func booleanInteger(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
