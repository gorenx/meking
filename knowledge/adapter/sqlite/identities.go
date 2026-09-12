package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
)

func (database *Database) EntityID(
	ctx context.Context,
	identity knowledge.EntityIdentity,
) (knowledge.EntityID, bool, error) {
	canonical, err := knowledge.NewEntityIdentity(identity.Title, identity.Type)
	if err != nil || canonical != identity {
		return "", false, fmt.Errorf("%w: Entity identity is not canonical", knowledge.ErrInvalidChange)
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return "", false, err
	}
	stored, err := queries.GetEntityID(ctx, db.GetEntityIDParams{
		ZoneID: queries.zoneID, Title: identity.Title, EntityType: identity.Type,
	})
	return restoredEntityID(stored, err)
}

func (database *Database) RelationID(
	ctx context.Context,
	key knowledge.RelationKey,
) (knowledge.RelationID, bool, error) {
	canonical, err := knowledge.NewRelationKey(key.SourceEntityID, key.TargetEntityID, key.Type)
	if err != nil || canonical != key {
		return "", false, fmt.Errorf("%w: Relation identity is not canonical", knowledge.ErrInvalidChange)
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return "", false, err
	}
	stored, err := queries.GetRelationID(ctx, db.GetRelationIDParams{
		ZoneID: queries.zoneID, SourceEntityID: string(key.SourceEntityID),
		TargetEntityID: string(key.TargetEntityID), RelationType: key.Type,
	})
	return restoredRelationID(stored, err)
}

func (database *Database) ClaimID(
	ctx context.Context,
	identity knowledge.ClaimIdentity,
) (knowledge.ClaimID, bool, error) {
	canonical, err := knowledge.NewClaimIdentity(identity.Subject, identity.Type)
	if err != nil || canonical.Type != identity.Type || !knowledge.SameSubject(canonical.Subject, identity.Subject) {
		return "", false, fmt.Errorf("%w: Claim identity is not canonical", knowledge.ErrInvalidChange)
	}
	subjectKind, subjectID, err := persistedSubject(identity.Subject)
	if err != nil {
		return "", false, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return "", false, err
	}
	stored, err := queries.GetClaimID(ctx, db.GetClaimIDParams{
		ZoneID: queries.zoneID, SubjectKind: subjectKind, SubjectID: subjectID, ClaimType: identity.Type,
	})
	return restoredClaimID(stored, err)
}

func (database *Database) ReserveEntity(
	ctx context.Context,
	id knowledge.EntityID,
	identity knowledge.EntityIdentity,
) error {
	if err := knowledge.ValidateEntityID(id); err != nil {
		return err
	}
	canonical, err := knowledge.NewEntityIdentity(identity.Title, identity.Type)
	if err != nil || canonical != identity {
		return fmt.Errorf("%w: Entity identity is not canonical", knowledge.ErrInvalidChange)
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	err = queries.CreateEntityIdentity(ctx, db.CreateEntityIdentityParams{
		ZoneID:     queries.zoneID,
		EntityID:   string(id),
		Title:      identity.Title,
		EntityType: identity.Type,
	})
	return identityWriteError("Entity", id, err)
}

func (database *Database) ReserveRelation(
	ctx context.Context,
	id knowledge.RelationID,
	key knowledge.RelationKey,
) error {
	if err := knowledge.ValidateRelationID(id); err != nil {
		return err
	}
	canonical, err := knowledge.NewRelationKey(key.SourceEntityID, key.TargetEntityID, key.Type)
	if err != nil || canonical != key {
		return fmt.Errorf("%w: Relation identity is not canonical", knowledge.ErrInvalidChange)
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	endpoints := []struct {
		role string
		id   knowledge.EntityID
	}{
		{
			role: "source",
			id:   key.SourceEntityID,
		},
		{
			role: "target",
			id:   key.TargetEntityID,
		},
	}
	for _, endpoint := range endpoints {
		known, err := queries.HasEntityIdentity(ctx, db.HasEntityIdentityParams{
			ZoneID:   queries.zoneID,
			EntityID: string(endpoint.id),
		})
		if err != nil {
			return classifySQLite("read Relation "+endpoint.role+" identity", err)
		}
		if known != 1 {
			return fmt.Errorf("%w: Relation %s Entity %q is unknown", knowledge.ErrInvalidChange, endpoint.role, endpoint.id)
		}
	}
	err = queries.CreateRelationIdentity(ctx, db.CreateRelationIdentityParams{
		ZoneID:         queries.zoneID,
		RelationID:     string(id),
		SourceEntityID: string(key.SourceEntityID),
		TargetEntityID: string(key.TargetEntityID),
		RelationType:   key.Type,
	})
	return identityWriteError("Relation", id, err)
}

func (database *Database) ReserveClaim(
	ctx context.Context,
	id knowledge.ClaimID,
	identity knowledge.ClaimIdentity,
) error {
	if err := knowledge.ValidateClaimID(id); err != nil {
		return err
	}
	canonical, err := knowledge.NewClaimIdentity(identity.Subject, identity.Type)
	if err != nil || canonical.Type != identity.Type || !knowledge.SameSubject(canonical.Subject, identity.Subject) {
		return fmt.Errorf("%w: Claim identity is not canonical", knowledge.ErrInvalidChange)
	}
	subjectKind, subjectID, err := persistedSubject(identity.Subject)
	if err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	var known int64
	if subjectKind == "entity" {
		known, err = queries.HasEntityIdentity(ctx, db.HasEntityIdentityParams{
			ZoneID:   queries.zoneID,
			EntityID: subjectID,
		})
	} else {
		known, err = queries.HasRelationIdentity(ctx, db.HasRelationIdentityParams{
			ZoneID:     queries.zoneID,
			RelationID: subjectID,
		})
	}
	if err != nil {
		return classifySQLite("read Claim Subject identity", err)
	}
	if known != 1 {
		return fmt.Errorf("%w: Claim Subject %s %q is unknown", knowledge.ErrInvalidChange, subjectKind, subjectID)
	}
	err = queries.CreateClaimIdentity(ctx, db.CreateClaimIdentityParams{
		ZoneID: queries.zoneID, ClaimID: string(id), SubjectKind: subjectKind,
		SubjectID: subjectID, ClaimType: identity.Type,
	})
	return identityWriteError("Claim", id, err)
}

func (database *Database) EntityIdentity(
	ctx context.Context,
	id knowledge.EntityID,
) (knowledge.EntityIdentity, bool, error) {
	if err := knowledge.ValidateEntityID(id); err != nil {
		return knowledge.EntityIdentity{}, false, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return knowledge.EntityIdentity{}, false, err
	}
	row, err := queries.GetEntityIdentity(ctx, db.GetEntityIdentityParams{
		ZoneID:   queries.zoneID,
		EntityID: string(id),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.EntityIdentity{}, false, nil
	}
	if err != nil {
		return knowledge.EntityIdentity{}, false, classifySQLite("read Entity identity", err)
	}
	return knowledge.EntityIdentity{
		Title: row.Title,
		Type:  row.EntityType,
	}, true, nil
}

func (database *Database) RelationKey(
	ctx context.Context,
	id knowledge.RelationID,
) (knowledge.RelationKey, bool, error) {
	if err := knowledge.ValidateRelationID(id); err != nil {
		return knowledge.RelationKey{}, false, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return knowledge.RelationKey{}, false, err
	}
	row, err := queries.GetRelationIdentity(ctx, db.GetRelationIdentityParams{
		ZoneID:     queries.zoneID,
		RelationID: string(id),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.RelationKey{}, false, nil
	}
	if err != nil {
		return knowledge.RelationKey{}, false, classifySQLite("read Relation identity", err)
	}
	return knowledge.RelationKey{
		SourceEntityID: knowledge.EntityID(row.SourceEntityID),
		TargetEntityID: knowledge.EntityID(row.TargetEntityID), Type: row.RelationType,
	}, true, nil
}

func (database *Database) ClaimIdentity(
	ctx context.Context,
	id knowledge.ClaimID,
) (knowledge.ClaimIdentity, bool, error) {
	if err := knowledge.ValidateClaimID(id); err != nil {
		return knowledge.ClaimIdentity{}, false, err
	}
	queries, err := database.reader(ctx)
	if err != nil {
		return knowledge.ClaimIdentity{}, false, err
	}
	row, err := queries.GetClaimIdentity(ctx, db.GetClaimIdentityParams{
		ZoneID:  queries.zoneID,
		ClaimID: string(id),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.ClaimIdentity{}, false, nil
	}
	if err != nil {
		return knowledge.ClaimIdentity{}, false, classifySQLite("read Claim identity", err)
	}
	subject, err := restoredSubject(row.SubjectKind, row.SubjectID)
	if err != nil {
		return knowledge.ClaimIdentity{}, false, err
	}
	return knowledge.ClaimIdentity{
		Subject: subject,
		Type:    row.ClaimType,
	}, true, nil
}

func identityWriteError(kind string, id knowledge.ObjectRef, err error) error {
	if constraint(err) {
		return fmt.Errorf("%w: %s %q or its identity already exists", knowledge.ErrIdentityConflict, kind, id.ObjectID())
	}
	return classifySQLite("reserve "+kind+" identity", err)
}

func restoredEntityID(stored string, err error) (knowledge.EntityID, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, classifySQLite("read Entity identity", err)
	}
	id := knowledge.EntityID(stored)
	if err := knowledge.ValidateEntityID(id); err != nil {
		return "", false, fmt.Errorf("%w: stored Entity identity %q: %v", knowledge.ErrDataIntegrity, stored, err)
	}
	return id, true, nil
}

func restoredRelationID(stored string, err error) (knowledge.RelationID, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, classifySQLite("read Relation identity", err)
	}
	id := knowledge.RelationID(stored)
	if err := knowledge.ValidateRelationID(id); err != nil {
		return "", false, fmt.Errorf("%w: stored Relation identity %q: %v", knowledge.ErrDataIntegrity, stored, err)
	}
	return id, true, nil
}

func restoredClaimID(stored string, err error) (knowledge.ClaimID, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, classifySQLite("read Claim identity", err)
	}
	id := knowledge.ClaimID(stored)
	if err := knowledge.ValidateClaimID(id); err != nil {
		return "", false, fmt.Errorf("%w: stored Claim identity %q: %v", knowledge.ErrDataIntegrity, stored, err)
	}
	return id, true, nil
}

func persistedSubject(subject knowledge.Subject) (string, string, error) {
	switch subject := subject.(type) {
	case knowledge.EntityID:
		if err := knowledge.ValidateEntityID(subject); err != nil {
			return "", "", err
		}
		return "entity", string(subject), nil
	case knowledge.RelationID:
		if err := knowledge.ValidateRelationID(subject); err != nil {
			return "", "", err
		}
		return "relation", string(subject), nil
	default:
		return "", "", fmt.Errorf("%w: unsupported Claim Subject %T", knowledge.ErrInvalidChange, subject)
	}
}

func restoredSubject(kind string, id string) (knowledge.Subject, error) {
	switch kind {
	case "entity":
		return knowledge.NewEntitySubject(knowledge.EntityID(id))
	case "relation":
		return knowledge.NewRelationSubject(knowledge.RelationID(id))
	default:
		return nil, fmt.Errorf("%w: stored Claim Subject kind %q", knowledge.ErrDataIntegrity, kind)
	}
}
