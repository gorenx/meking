package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/knowledge/zonemerger"
	"github.com/memoria-space/meking/zone"
)

func (database *Database) SaveEntityConflict(ctx context.Context, value zonemerger.EntityConflict) error {
	if err := validateEntityConflict(ctx, value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	stored, found, err := database.EntityConflict(ctx, value.ChildEntityID, value.ChildBase)
	if err != nil {
		return err
	}
	if found {
		if stored != value {
			return zonemerger.ErrConflictChanged
		}
		return nil
	}
	err = queries.SaveZoneEntityConflict(ctx, db.SaveZoneEntityConflictParams{
		ChildZoneID:          queries.zoneID,
		ChildEntityID:        string(value.ChildEntityID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentEntityID:       string(value.ParentEntityID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
	})
	if constraint(err) {
		return zonemerger.ErrConflictChanged
	}
	return classifySQLite("save Zone Entity conflict", err)
}

func (database *Database) SaveRelationConflict(ctx context.Context, value zonemerger.RelationConflict) error {
	if err := validateRelationConflict(ctx, value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	stored, found, err := database.RelationConflict(ctx, value.ChildRelationID, value.ChildBase)
	if err != nil {
		return err
	}
	if found {
		if stored != value {
			return zonemerger.ErrConflictChanged
		}
		return nil
	}
	err = queries.SaveZoneRelationConflict(ctx, db.SaveZoneRelationConflictParams{
		ChildZoneID:          queries.zoneID,
		ChildRelationID:      string(value.ChildRelationID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentRelationID:     string(value.ParentRelationID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
	})
	if constraint(err) {
		return zonemerger.ErrConflictChanged
	}
	return classifySQLite("save Zone Relation conflict", err)
}

func (database *Database) SaveClaimConflict(ctx context.Context, value zonemerger.ClaimConflict) error {
	if err := validateClaimConflict(ctx, value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	stored, found, err := database.ClaimConflict(ctx, value.ChildClaimID, value.ChildBase)
	if err != nil {
		return err
	}
	if found {
		if stored != value {
			return zonemerger.ErrConflictChanged
		}
		return nil
	}
	err = queries.SaveZoneClaimConflict(ctx, db.SaveZoneClaimConflictParams{
		ChildZoneID:          queries.zoneID,
		ChildClaimID:         string(value.ChildClaimID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentClaimID:        string(value.ParentClaimID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
	})
	if constraint(err) {
		return zonemerger.ErrConflictChanged
	}
	return classifySQLite("save Zone Claim conflict", err)
}

func (database *Database) EntityConflict(ctx context.Context, childEntityID knowledge.EntityID, childBase knowledge.Version) (zonemerger.EntityConflict, bool, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return zonemerger.EntityConflict{}, false, err
	}
	row, err := queries.GetZoneEntityConflict(ctx, db.GetZoneEntityConflictParams{
		ChildZoneID:      queries.zoneID,
		ChildEntityID:    string(childEntityID),
		ChildBaseVersion: int64(childBase),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return zonemerger.EntityConflict{}, false, nil
	}
	if err != nil {
		return zonemerger.EntityConflict{}, false, classifySQLite("read Zone Entity conflict", err)
	}
	hash, err := storedHash(row.CandidateContentHash)
	if err != nil {
		return zonemerger.EntityConflict{}, false, err
	}
	return zonemerger.EntityConflict{
		ChildZoneID:    zone.ID(queries.zoneID),
		ChildEntityID:  childEntityID,
		ChildBase:      childBase,
		ParentZoneID:   zone.ID(row.ParentZoneID),
		ParentEntityID: knowledge.EntityID(row.ParentEntityID),
		ParentVersion:  knowledge.Version(row.ParentVersion),
		CandidateHash:  hash,
	}, true, nil
}

func (database *Database) RelationConflict(ctx context.Context, childRelationID knowledge.RelationID, childBase knowledge.Version) (zonemerger.RelationConflict, bool, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return zonemerger.RelationConflict{}, false, err
	}
	row, err := queries.GetZoneRelationConflict(ctx, db.GetZoneRelationConflictParams{
		ChildZoneID:      queries.zoneID,
		ChildRelationID:  string(childRelationID),
		ChildBaseVersion: int64(childBase),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return zonemerger.RelationConflict{}, false, nil
	}
	if err != nil {
		return zonemerger.RelationConflict{}, false, classifySQLite("read Zone Relation conflict", err)
	}
	hash, err := storedHash(row.CandidateContentHash)
	if err != nil {
		return zonemerger.RelationConflict{}, false, err
	}
	return zonemerger.RelationConflict{
		ChildZoneID:      zone.ID(queries.zoneID),
		ChildRelationID:  childRelationID,
		ChildBase:        childBase,
		ParentZoneID:     zone.ID(row.ParentZoneID),
		ParentRelationID: knowledge.RelationID(row.ParentRelationID),
		ParentVersion:    knowledge.Version(row.ParentVersion),
		CandidateHash:    hash,
	}, true, nil
}

func (database *Database) ClaimConflict(ctx context.Context, childClaimID knowledge.ClaimID, childBase knowledge.Version) (zonemerger.ClaimConflict, bool, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return zonemerger.ClaimConflict{}, false, err
	}
	row, err := queries.GetZoneClaimConflict(ctx, db.GetZoneClaimConflictParams{
		ChildZoneID:      queries.zoneID,
		ChildClaimID:     string(childClaimID),
		ChildBaseVersion: int64(childBase),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return zonemerger.ClaimConflict{}, false, nil
	}
	if err != nil {
		return zonemerger.ClaimConflict{}, false, classifySQLite("read Zone Claim conflict", err)
	}
	hash, err := storedHash(row.CandidateContentHash)
	if err != nil {
		return zonemerger.ClaimConflict{}, false, err
	}
	return zonemerger.ClaimConflict{
		ChildZoneID:   zone.ID(queries.zoneID),
		ChildClaimID:  childClaimID,
		ChildBase:     childBase,
		ParentZoneID:  zone.ID(row.ParentZoneID),
		ParentClaimID: knowledge.ClaimID(row.ParentClaimID),
		ParentVersion: knowledge.Version(row.ParentVersion),
		CandidateHash: hash,
	}, true, nil
}

func (database *Database) ResolvedEntityConflict(ctx context.Context, resolutionSourceID string) (zonemerger.EntityConflict, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return zonemerger.EntityConflict{}, err
	}
	row, err := queries.GetResolvedZoneEntityConflict(ctx, db.GetResolvedZoneEntityConflictParams{
		ChildZoneID: queries.zoneID,
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return zonemerger.EntityConflict{}, knowledge.ErrNotFound
	}
	if err != nil {
		return zonemerger.EntityConflict{}, classifySQLite("read resolved Zone Entity conflict", err)
	}
	hash, err := storedHash(row.CandidateContentHash)
	if err != nil {
		return zonemerger.EntityConflict{}, err
	}
	return zonemerger.EntityConflict{
		ChildZoneID:      zone.ID(queries.zoneID),
		ChildEntityID:    knowledge.EntityID(row.ChildEntityID),
		ChildBase:        knowledge.Version(row.ChildBaseVersion),
		ParentZoneID:     zone.ID(row.ParentZoneID),
		ParentEntityID:   knowledge.EntityID(row.ParentEntityID),
		ParentVersion:    knowledge.Version(row.ParentVersion),
		CandidateHash:    hash,
		ParentReconciled: row.ParentReconciled == 1,
	}, nil
}

func (database *Database) ResolvedRelationConflict(ctx context.Context, resolutionSourceID string) (zonemerger.RelationConflict, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return zonemerger.RelationConflict{}, err
	}
	row, err := queries.GetResolvedZoneRelationConflict(ctx, db.GetResolvedZoneRelationConflictParams{
		ChildZoneID: queries.zoneID,
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return zonemerger.RelationConflict{}, knowledge.ErrNotFound
	}
	if err != nil {
		return zonemerger.RelationConflict{}, classifySQLite("read resolved Zone Relation conflict", err)
	}
	hash, err := storedHash(row.CandidateContentHash)
	if err != nil {
		return zonemerger.RelationConflict{}, err
	}
	return zonemerger.RelationConflict{
		ChildZoneID:      zone.ID(queries.zoneID),
		ChildRelationID:  knowledge.RelationID(row.ChildRelationID),
		ChildBase:        knowledge.Version(row.ChildBaseVersion),
		ParentZoneID:     zone.ID(row.ParentZoneID),
		ParentRelationID: knowledge.RelationID(row.ParentRelationID),
		ParentVersion:    knowledge.Version(row.ParentVersion),
		CandidateHash:    hash,
		ParentReconciled: row.ParentReconciled == 1,
	}, nil
}

func (database *Database) ResolvedClaimConflict(ctx context.Context, resolutionSourceID string) (zonemerger.ClaimConflict, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return zonemerger.ClaimConflict{}, err
	}
	row, err := queries.GetResolvedZoneClaimConflict(ctx, db.GetResolvedZoneClaimConflictParams{
		ChildZoneID: queries.zoneID,
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return zonemerger.ClaimConflict{}, knowledge.ErrNotFound
	}
	if err != nil {
		return zonemerger.ClaimConflict{}, classifySQLite("read resolved Zone Claim conflict", err)
	}
	hash, err := storedHash(row.CandidateContentHash)
	if err != nil {
		return zonemerger.ClaimConflict{}, err
	}
	return zonemerger.ClaimConflict{
		ChildZoneID:      zone.ID(queries.zoneID),
		ChildClaimID:     knowledge.ClaimID(row.ChildClaimID),
		ChildBase:        knowledge.Version(row.ChildBaseVersion),
		ParentZoneID:     zone.ID(row.ParentZoneID),
		ParentClaimID:    knowledge.ClaimID(row.ParentClaimID),
		ParentVersion:    knowledge.Version(row.ParentVersion),
		CandidateHash:    hash,
		ParentReconciled: row.ParentReconciled == 1,
	}, nil
}

func (database *Database) ResolveEntityConflict(ctx context.Context, value zonemerger.EntityConflict, resolutionSourceID string) error {
	if err := validateEntityConflict(ctx, value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	rows, err := queries.ResolveZoneEntityConflict(ctx, db.ResolveZoneEntityConflictParams{
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
		ChildZoneID:          queries.zoneID,
		ChildEntityID:        string(value.ChildEntityID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentEntityID:       string(value.ParentEntityID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
	})
	return changed("resolve Zone Entity conflict", rows, err)
}

func (database *Database) ResolveRelationConflict(ctx context.Context, value zonemerger.RelationConflict, resolutionSourceID string) error {
	if err := validateRelationConflict(ctx, value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	rows, err := queries.ResolveZoneRelationConflict(ctx, db.ResolveZoneRelationConflictParams{
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
		ChildZoneID:          queries.zoneID,
		ChildRelationID:      string(value.ChildRelationID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentRelationID:     string(value.ParentRelationID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
	})
	return changed("resolve Zone Relation conflict", rows, err)
}

func (database *Database) ResolveClaimConflict(ctx context.Context, value zonemerger.ClaimConflict, resolutionSourceID string) error {
	if err := validateClaimConflict(ctx, value); err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	rows, err := queries.ResolveZoneClaimConflict(ctx, db.ResolveZoneClaimConflictParams{
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
		ChildZoneID:          queries.zoneID,
		ChildClaimID:         string(value.ChildClaimID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentClaimID:        string(value.ParentClaimID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
	})
	return changed("resolve Zone Claim conflict", rows, err)
}

func (database *Database) ReconcileEntityConflict(ctx context.Context, value zonemerger.EntityConflict, resolutionSourceID string) error {
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	rows, err := queries.ReconcileZoneEntityConflict(ctx, db.ReconcileZoneEntityConflictParams{
		ChildZoneID:          queries.zoneID,
		ChildEntityID:        string(value.ChildEntityID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentEntityID:       string(value.ParentEntityID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
	})
	return changed("reconcile Zone Entity conflict", rows, err)
}

func (database *Database) ReconcileRelationConflict(ctx context.Context, value zonemerger.RelationConflict, resolutionSourceID string) error {
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	rows, err := queries.ReconcileZoneRelationConflict(ctx, db.ReconcileZoneRelationConflictParams{
		ChildZoneID:          queries.zoneID,
		ChildRelationID:      string(value.ChildRelationID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentRelationID:     string(value.ParentRelationID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
	})
	return changed("reconcile Zone Relation conflict", rows, err)
}

func (database *Database) ReconcileClaimConflict(ctx context.Context, value zonemerger.ClaimConflict, resolutionSourceID string) error {
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	rows, err := queries.ReconcileZoneClaimConflict(ctx, db.ReconcileZoneClaimConflictParams{
		ChildZoneID:          queries.zoneID,
		ChildClaimID:         string(value.ChildClaimID),
		ChildBaseVersion:     int64(value.ChildBase),
		ParentZoneID:         string(value.ParentZoneID),
		ParentClaimID:        string(value.ParentClaimID),
		ParentVersion:        int64(value.ParentVersion),
		CandidateContentHash: value.CandidateHash[:],
		ResolutionSourceID: sql.NullString{
			String: resolutionSourceID,
			Valid:  true,
		},
	})
	return changed("reconcile Zone Claim conflict", rows, err)
}

func changed(operation string, rows int64, err error) error {
	if err != nil {
		return classifySQLite(operation, err)
	}
	if rows != 1 {
		return zonemerger.ErrConflictChanged
	}
	return nil
}

func validateEntityConflict(ctx context.Context, value zonemerger.EntityConflict) error {
	if err := validateConflictZones(ctx, value.ChildZoneID, value.ParentZoneID); err != nil {
		return err
	}
	if err := knowledge.ValidateEntityID(value.ChildEntityID); err != nil {
		return err
	}
	if err := knowledge.ValidateEntityID(value.ParentEntityID); err != nil {
		return err
	}
	return validateConflictVersions(value.ChildBase, value.ParentVersion)
}

func validateRelationConflict(ctx context.Context, value zonemerger.RelationConflict) error {
	if err := validateConflictZones(ctx, value.ChildZoneID, value.ParentZoneID); err != nil {
		return err
	}
	if err := knowledge.ValidateRelationID(value.ChildRelationID); err != nil {
		return err
	}
	if err := knowledge.ValidateRelationID(value.ParentRelationID); err != nil {
		return err
	}
	return validateConflictVersions(value.ChildBase, value.ParentVersion)
}

func validateClaimConflict(ctx context.Context, value zonemerger.ClaimConflict) error {
	if err := validateConflictZones(ctx, value.ChildZoneID, value.ParentZoneID); err != nil {
		return err
	}
	if err := knowledge.ValidateClaimID(value.ChildClaimID); err != nil {
		return err
	}
	if err := knowledge.ValidateClaimID(value.ParentClaimID); err != nil {
		return err
	}
	return validateConflictVersions(value.ChildBase, value.ParentVersion)
}

func validateConflictZones(ctx context.Context, child zone.ID, parent zone.ID) error {
	current, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	if _, err := zone.ParseID(string(child)); err != nil {
		return err
	}
	if _, err := zone.ParseID(string(parent)); err != nil {
		return err
	}
	if child != current || child == parent {
		return zonemerger.ErrInvalidMerge
	}
	return nil
}

func validateConflictVersions(child knowledge.Version, parent knowledge.Version) error {
	if child == 0 || parent == 0 {
		return zonemerger.ErrInvalidMerge
	}
	return nil
}
