package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/community/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/zone"
)

var _ community.StructureStore = (*Store)(nil)
var _ community.StructureStateStore = (*Store)(nil)

type storedStructure struct {
	ID              string
	CommunitySetID  string
	CorporaID       string
	KnowledgeDigest string
	CreatedAt       string
}

func (store *Store) SaveStructure(ctx context.Context, value community.Structure) error {
	restored, err := community.RestoreStructure(value)
	if err != nil {
		return err
	}
	digest, err := restored.Knowledge.Digest()
	if err != nil {
		return err
	}
	return store.write(ctx, func(statements statements) error {
		inserted, err := statements.queries.CreateStructure(ctx, db.CreateStructureParams{
			ZoneID:          statements.zoneID,
			ID:              string(restored.ID),
			CommunitySetID:  string(restored.CommunitySetID),
			CorporaID:       restored.CorporaID,
			KnowledgeDigest: digest,
			CreatedAt:       restored.CreatedAt.Format(time.RFC3339Nano),
		})
		if err != nil {
			return classifySQLite("save Community Structure", err)
		}
		if inserted == 1 {
			if err := saveStructureVersions(ctx, statements, restored); err != nil {
				return err
			}
			return nil
		}
		existing, err := loadStructure(ctx, statements.queries, statements.zoneID, restored.ID)
		if err != nil || !sameStructure(existing, restored) {
			return community.ErrStructureConflict
		}
		return nil
	})
}

func saveStructureVersions(
	ctx context.Context,
	statements statements,
	structure community.Structure,
) error {
	for _, reference := range structure.Knowledge.Entities {
		if err := statements.queries.AddStructureEntityVersion(ctx, db.AddStructureEntityVersionParams{
			ZoneID:      statements.zoneID,
			StructureID: string(structure.ID),
			EntityID:    string(reference.ID),
			Version:     int64(reference.Version),
		}); err != nil {
			return classifySQLite("save Community Structure Entity Version", err)
		}
	}
	for _, reference := range structure.Knowledge.Relations {
		if err := statements.queries.AddStructureRelationVersion(ctx, db.AddStructureRelationVersionParams{
			ZoneID:      statements.zoneID,
			StructureID: string(structure.ID),
			RelationID:  string(reference.ID),
			Version:     int64(reference.Version),
		}); err != nil {
			return classifySQLite("save Community Structure Relation Version", err)
		}
	}
	for _, reference := range structure.Knowledge.Claims {
		if err := statements.queries.AddStructureClaimVersion(ctx, db.AddStructureClaimVersionParams{
			ZoneID:      statements.zoneID,
			StructureID: string(structure.ID),
			ClaimID:     string(reference.ID),
			Version:     int64(reference.Version),
		}); err != nil {
			return classifySQLite("save Community Structure Claim Version", err)
		}
	}
	return nil
}

func (store *Store) LoadStructure(ctx context.Context, id community.StructureID) (community.Structure, error) {
	if _, err := community.RestoreStructureID(id); err != nil {
		return community.Structure{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return community.Structure{}, err
	}
	return loadStructure(ctx, newGlobalStatements(store.database), string(zoneID), id)
}

func (store *Store) LoadStructureAt(
	ctx context.Context,
	corporaID string,
	versions knowledge.Manifest,
) (community.Structure, error) {
	if corporaID == "" {
		return community.Structure{}, community.ErrInvalidStructure
	}
	digest, err := versions.Digest()
	if err != nil {
		return community.Structure{}, community.ErrInvalidStructure
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return community.Structure{}, err
	}
	queries := newGlobalStatements(store.database)
	row, err := queries.GetStructureAt(ctx, db.GetStructureAtParams{
		ZoneID:          string(zoneID),
		CorporaID:       corporaID,
		KnowledgeDigest: digest,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return community.Structure{}, community.ErrStructureNotFound
	}
	if err != nil {
		return community.Structure{}, classifySQLite("read Community Structure boundary", err)
	}
	structure, err := restoreStructure(ctx, queries, string(zoneID), storedStructure{
		ID:              row.ID,
		CommunitySetID:  row.CommunitySetID,
		CorporaID:       row.CorporaID,
		KnowledgeDigest: row.KnowledgeDigest,
		CreatedAt:       row.CreatedAt,
	})
	if err != nil {
		return community.Structure{}, err
	}
	if !structure.Knowledge.Equal(versions) {
		return community.Structure{}, community.ErrStructureConflict
	}
	return structure, nil
}

func (store *Store) LoadStructureState(ctx context.Context) (community.StructureState, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return community.StructureState{}, err
	}
	row, err := newGlobalStatements(store.database).GetStructureState(ctx, string(zoneID))
	if errors.Is(err, sql.ErrNoRows) {
		return community.StructureState{}, nil
	}
	if err != nil {
		return community.StructureState{}, classifySQLite("read Community Structure state", err)
	}
	return restoreStructureState(row)
}

func (store *Store) SaveStructureState(
	ctx context.Context,
	expected community.StructureState,
	state community.StructureState,
) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if state.StructureID == "" {
		return community.ErrInvalidStructure
	}
	return store.write(ctx, func(statements statements) error {
		parameters := db.CreateStructureStateParams{
			ZoneID:                 statements.zoneID,
			StructureID:            string(state.StructureID),
			PendingRelationChanges: int64(state.PendingRelationChanges),
		}
		if expected.StructureID == "" {
			inserted, err := statements.queries.CreateStructureState(ctx, parameters)
			if err != nil {
				return classifySQLite("create Community Structure state", err)
			}
			if inserted == 1 {
				return nil
			}
		}
		updated, err := statements.queries.AdvanceStructureState(ctx, db.AdvanceStructureStateParams{
			StructureID:            string(state.StructureID),
			PendingRelationChanges: int64(state.PendingRelationChanges),
			ZoneID:                 statements.zoneID,
			ExpectedStructureID:    string(expected.StructureID),
		})
		if err != nil {
			return classifySQLite("advance Community Structure state", err)
		}
		if updated != 1 {
			return community.ErrStructureConflict
		}
		return nil
	})
}

func loadStructure(
	ctx context.Context,
	queries *db.Queries,
	zoneID string,
	id community.StructureID,
) (community.Structure, error) {
	row, err := queries.GetStructure(ctx, db.GetStructureParams{
		ZoneID: zoneID,
		ID:     string(id),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return community.Structure{}, community.ErrStructureNotFound
	}
	if err != nil {
		return community.Structure{}, classifySQLite("read Community Structure", err)
	}
	return restoreStructure(ctx, queries, zoneID, storedStructure{
		ID:              row.ID,
		CommunitySetID:  row.CommunitySetID,
		CorporaID:       row.CorporaID,
		KnowledgeDigest: row.KnowledgeDigest,
		CreatedAt:       row.CreatedAt,
	})
}

func restoreStructure(
	ctx context.Context,
	queries *db.Queries,
	zoneID string,
	stored storedStructure,
) (community.Structure, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, stored.CreatedAt)
	if err != nil {
		return community.Structure{}, community.ErrCommunityDataIntegrity
	}
	versions, err := loadStructureVersions(ctx, queries, zoneID, stored.ID)
	if err != nil {
		return community.Structure{}, err
	}
	digest, err := versions.Digest()
	if err != nil || digest != stored.KnowledgeDigest {
		return community.Structure{}, community.ErrCommunityDataIntegrity
	}
	value, err := community.RestoreStructure(community.Structure{
		ID:             community.StructureID(stored.ID),
		CommunitySetID: community.CommunitySetID(stored.CommunitySetID),
		CorporaID:      stored.CorporaID,
		Knowledge:      versions,
		CreatedAt:      createdAt,
	})
	if err != nil {
		return community.Structure{}, fmt.Errorf("%w: %v", community.ErrCommunityDataIntegrity, err)
	}
	return value, nil
}

func loadStructureVersions(
	ctx context.Context,
	queries *db.Queries,
	zoneID string,
	structureID string,
) (knowledge.Manifest, error) {
	entities, err := queries.ListStructureEntityVersions(ctx, db.ListStructureEntityVersionsParams{
		ZoneID:      zoneID,
		StructureID: structureID,
	})
	if err != nil {
		return knowledge.Manifest{}, classifySQLite("read Community Structure Entity Versions", err)
	}
	relations, err := queries.ListStructureRelationVersions(ctx, db.ListStructureRelationVersionsParams{
		ZoneID:      zoneID,
		StructureID: structureID,
	})
	if err != nil {
		return knowledge.Manifest{}, classifySQLite("read Community Structure Relation Versions", err)
	}
	claims, err := queries.ListStructureClaimVersions(ctx, db.ListStructureClaimVersionsParams{
		ZoneID:      zoneID,
		StructureID: structureID,
	})
	if err != nil {
		return knowledge.Manifest{}, classifySQLite("read Community Structure Claim Versions", err)
	}
	result := knowledge.Manifest{
		Entities:  make([]knowledge.Reference[knowledge.EntityID], len(entities)),
		Relations: make([]knowledge.Reference[knowledge.RelationID], len(relations)),
		Claims:    make([]knowledge.Reference[knowledge.ClaimID], len(claims)),
	}
	for index, row := range entities {
		result.Entities[index] = knowledge.Reference[knowledge.EntityID]{
			ID:      knowledge.EntityID(row.EntityID),
			Version: knowledge.Version(row.Version),
		}
	}
	for index, row := range relations {
		result.Relations[index] = knowledge.Reference[knowledge.RelationID]{
			ID:      knowledge.RelationID(row.RelationID),
			Version: knowledge.Version(row.Version),
		}
	}
	for index, row := range claims {
		result.Claims[index] = knowledge.Reference[knowledge.ClaimID]{
			ID:      knowledge.ClaimID(row.ClaimID),
			Version: knowledge.Version(row.Version),
		}
	}
	if err := result.Validate(); err != nil {
		return knowledge.Manifest{}, community.ErrCommunityDataIntegrity
	}
	return result, nil
}

func restoreStructureState(row db.GetStructureStateRow) (community.StructureState, error) {
	if row.PendingRelationChanges < 0 {
		return community.StructureState{}, community.ErrCommunityDataIntegrity
	}
	state := community.StructureState{
		StructureID:            community.StructureID(row.StructureID),
		PendingRelationChanges: uint64(row.PendingRelationChanges),
	}
	if err := state.Validate(); err != nil {
		return community.StructureState{}, fmt.Errorf("%w: %v", community.ErrCommunityDataIntegrity, err)
	}
	return state, nil
}

func sameStructure(left, right community.Structure) bool {
	return left.ID == right.ID &&
		left.CommunitySetID == right.CommunitySetID &&
		left.CorporaID == right.CorporaID &&
		left.CreatedAt.Equal(right.CreatedAt) &&
		left.Knowledge.Equal(right.Knowledge)
}
