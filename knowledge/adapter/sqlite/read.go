package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/zone"
)

type currentView struct {
	connection *sql.Conn
	statements statements
	closeOnce  sync.Once
	closeErr   error
}

type identityReader struct {
	statements statements
}

type entityReader struct {
	statements statements
}

type relationReader struct {
	statements statements
}

type claimReader struct {
	statements statements
}

func (database *Database) OpenCurrent(ctx context.Context) (_ knowledge.View, resultErr error) {
	if database == nil || database.database == nil {
		return nil, errors.New("open current Knowledge View: Database is not configured")
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, err
	}
	connection, err := database.database.Conn(ctx)
	if err != nil {
		return nil, classifySQLite("reserve current Knowledge View connection", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, connection.Close())
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN"); err != nil {
		return nil, classifySQLite("begin current Knowledge View", err)
	}
	open := true
	defer func() {
		if resultErr != nil && open {
			_, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
			resultErr = errors.Join(resultErr, rollbackErr)
		}
	}()
	var actualSchemaVersion int
	if err := connection.QueryRowContext(ctx, "SELECT version FROM knowledge_schema WHERE id = 1").Scan(&actualSchemaVersion); err != nil {
		return nil, classifySQLite("fix current Knowledge View", err)
	}
	if actualSchemaVersion != schemaVersion {
		return nil, fmt.Errorf("%w: current Knowledge View schema is %d", knowledge.ErrDataIntegrity, actualSchemaVersion)
	}
	open = false
	return &currentView{
		connection: connection,
		statements: newStatements(connection, string(zoneID)),
	}, nil
}

func (view *currentView) Identities() knowledge.IdentityReader {
	return identityReader{
		statements: view.statements,
	}
}

func (view *currentView) Entities() knowledge.VersionReader[knowledge.Entity, knowledge.EntityID] {
	return entityReader{
		statements: view.statements,
	}
}

func (view *currentView) Relations() knowledge.VersionReader[knowledge.Relation, knowledge.RelationID] {
	return relationReader{
		statements: view.statements,
	}
}

func (view *currentView) Claims() knowledge.VersionReader[knowledge.Claim, knowledge.ClaimID] {
	return claimReader{
		statements: view.statements,
	}
}

func (view *currentView) Close() error {
	if view == nil || view.connection == nil {
		return nil
	}
	view.closeOnce.Do(func() {
		_, rollbackErr := view.connection.ExecContext(context.Background(), "ROLLBACK")
		view.closeErr = errors.Join(rollbackErr, view.connection.Close())
	})
	return view.closeErr
}

func (reader identityReader) Entity(
	ctx context.Context,
	identity knowledge.EntityIdentity,
) (knowledge.KnowledgeVersion[knowledge.Entity], bool, error) {
	id, err := reader.statements.GetEntityID(ctx, db.GetEntityIDParams{
		ZoneID:     reader.statements.zoneID,
		Title:      identity.Title,
		EntityType: identity.Type,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, nil
	}
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, false, classifySQLite("read Entity identity", err)
	}
	return readCurrentEntity(ctx, reader.statements, knowledge.EntityID(id))
}

func (reader identityReader) FindEntities(
	ctx context.Context,
	title string,
	entityType string,
	limit int,
) (knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID], error) {
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]{}, err
	}
	rows, err := reader.statements.FindEntities(ctx,
		db.FindEntitiesParams{
			ZoneID:     reader.statements.zoneID,
			Title:      title,
			EntityType: entityType,
			PageSize:   pageSize,
		},
	)
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]{}, classifySQLite("find Entities", err)
	}
	items := make([]knowledge.Reference[knowledge.EntityID], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		items = append(items, knowledge.Reference[knowledge.EntityID]{
			ID:      knowledge.EntityID(row.EntityID),
			Version: knowledge.Version(row.Version),
		})
	}
	page := knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]{
		Items:   items,
		HasMore: len(rows) > limit,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].ID
	}
	return page, nil
}

func (reader identityReader) Relation(
	ctx context.Context,
	key knowledge.RelationKey,
) (knowledge.KnowledgeVersion[knowledge.Relation], bool, error) {
	id, err := reader.statements.GetRelationID(ctx, db.GetRelationIDParams{
		ZoneID:         reader.statements.zoneID,
		SourceEntityID: string(key.SourceEntityID),
		TargetEntityID: string(key.TargetEntityID),
		RelationType:   key.Type,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, nil
	}
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, false, classifySQLite("read Relation identity", err)
	}
	return readCurrentRelation(ctx, reader.statements, knowledge.RelationID(id))
}

func (reader identityReader) Claim(
	ctx context.Context,
	identity knowledge.ClaimIdentity,
) (knowledge.KnowledgeVersion[knowledge.Claim], bool, error) {
	subjectKind, subjectID, err := persistedSubject(identity.Subject)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, err
	}
	id, err := reader.statements.GetClaimID(ctx, db.GetClaimIDParams{
		ZoneID:      reader.statements.zoneID,
		SubjectKind: subjectKind,
		SubjectID:   subjectID,
		ClaimType:   identity.Type,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, nil
	}
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, false, classifySQLite("read Claim identity", err)
	}
	return readCurrentClaim(ctx, reader.statements, knowledge.ClaimID(id))
}

func (reader entityReader) Active(
	ctx context.Context,
	after knowledge.EntityID,
	limit int,
) (knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID], error) {
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]{}, err
	}
	rows, err := reader.statements.ListActiveEntityReferences(ctx, db.ListActiveEntityReferencesParams{
		ZoneID:        reader.statements.zoneID,
		AfterEntityID: string(after),
		PageSize:      pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]{}, classifySQLite("list active Entity references", err)
	}
	items := make([]knowledge.Reference[knowledge.EntityID], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		items = append(items, knowledge.Reference[knowledge.EntityID]{
			ID:      knowledge.EntityID(row.EntityID),
			Version: knowledge.Version(row.Version),
		})
	}
	return entityReferencePage(items, len(rows) > limit), nil
}

func (reader relationReader) Active(
	ctx context.Context,
	after knowledge.RelationID,
	limit int,
) (knowledge.Page[knowledge.Reference[knowledge.RelationID], knowledge.RelationID], error) {
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.RelationID], knowledge.RelationID]{}, err
	}
	rows, err := reader.statements.ListActiveRelationReferences(ctx, db.ListActiveRelationReferencesParams{
		ZoneID:          reader.statements.zoneID,
		AfterRelationID: string(after),
		PageSize:        pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.RelationID], knowledge.RelationID]{}, classifySQLite("list active Relation references", err)
	}
	items := make([]knowledge.Reference[knowledge.RelationID], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		items = append(items, knowledge.Reference[knowledge.RelationID]{
			ID:      knowledge.RelationID(row.RelationID),
			Version: knowledge.Version(row.Version),
		})
	}
	return relationReferencePage(items, len(rows) > limit), nil
}

func (reader claimReader) Active(
	ctx context.Context,
	after knowledge.ClaimID,
	limit int,
) (knowledge.Page[knowledge.Reference[knowledge.ClaimID], knowledge.ClaimID], error) {
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.ClaimID], knowledge.ClaimID]{}, err
	}
	rows, err := reader.statements.ListActiveClaimReferences(ctx, db.ListActiveClaimReferencesParams{
		ZoneID:       reader.statements.zoneID,
		AfterClaimID: string(after),
		PageSize:     pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.Reference[knowledge.ClaimID], knowledge.ClaimID]{}, classifySQLite("list active Claim references", err)
	}
	items := make([]knowledge.Reference[knowledge.ClaimID], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		items = append(items, knowledge.Reference[knowledge.ClaimID]{
			ID:      knowledge.ClaimID(row.ClaimID),
			Version: knowledge.Version(row.Version),
		})
	}
	return claimReferencePage(items, len(rows) > limit), nil
}

func (reader entityReader) Current(
	ctx context.Context,
	after knowledge.EntityID,
	limit int,
) (knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.EntityID], error) {
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.EntityID]{}, err
	}
	rows, err := reader.statements.ListCurrentEntities(ctx, db.ListCurrentEntitiesParams{
		ZoneID:        reader.statements.zoneID,
		AfterEntityID: string(after),
		PageSize:      pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.EntityID]{}, classifySQLite("list current Entities", err)
	}
	items := make([]knowledge.KnowledgeVersion[knowledge.Entity], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		version, found, err := restoreEntity(ctx, reader.statements, entityVersionRecord{
			id:          knowledge.EntityID(row.EntityID),
			title:       row.Title,
			entityType:  row.EntityType,
			version:     row.Version,
			deleted:     row.Deleted,
			hash:        row.ContentHash,
			description: row.Description,
		}, nil)
		if err != nil || !found {
			return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.EntityID]{}, storedVersionError("Entity", row.EntityID, err)
		}
		items = append(items, version)
	}
	return entityVersionPage(items, len(rows) > limit), nil
}

func (reader relationReader) Current(
	ctx context.Context,
	after knowledge.RelationID,
	limit int,
) (knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.RelationID], error) {
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.RelationID]{}, err
	}
	rows, err := reader.statements.ListCurrentRelations(ctx, db.ListCurrentRelationsParams{
		ZoneID:          reader.statements.zoneID,
		AfterRelationID: string(after),
		PageSize:        pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.RelationID]{}, classifySQLite("list current Relations", err)
	}
	items := make([]knowledge.KnowledgeVersion[knowledge.Relation], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		version, found, err := restoreRelation(relationVersionRecord{
			id:             knowledge.RelationID(row.RelationID),
			sourceEntityID: row.SourceEntityID,
			targetEntityID: row.TargetEntityID,
			relationType:   row.RelationType,
			version:        row.Version,
			deleted:        row.Deleted,
			hash:           row.ContentHash,
			description:    row.Description,
		}, nil)
		if err != nil || !found {
			return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.RelationID]{}, storedVersionError("Relation", row.RelationID, err)
		}
		items = append(items, version)
	}
	return relationVersionPage(items, len(rows) > limit), nil
}

func (reader claimReader) Current(
	ctx context.Context,
	after knowledge.ClaimID,
	limit int,
) (knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.ClaimID], error) {
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.ClaimID]{}, err
	}
	rows, err := reader.statements.ListCurrentClaims(ctx, db.ListCurrentClaimsParams{
		ZoneID:       reader.statements.zoneID,
		AfterClaimID: string(after),
		PageSize:     pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.ClaimID]{}, classifySQLite("list current Claims", err)
	}
	items := make([]knowledge.KnowledgeVersion[knowledge.Claim], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		version, found, err := restoreClaim(claimVersionRecord{
			id:          knowledge.ClaimID(row.ClaimID),
			subjectKind: row.SubjectKind,
			subjectID:   row.SubjectID,
			claimType:   row.ClaimType,
			version:     row.Version,
			deleted:     row.Deleted,
			hash:        row.ContentHash,
			description: row.Description,
		}, nil)
		if err != nil || !found {
			return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.ClaimID]{}, storedVersionError("Claim", row.ClaimID, err)
		}
		items = append(items, version)
	}
	return claimVersionPage(items, len(rows) > limit), nil
}

func (reader entityReader) History(
	ctx context.Context,
	entityID knowledge.EntityID,
	after knowledge.Version,
	limit int,
) (knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.Version], error) {
	if err := knowledge.ValidateEntityID(entityID); err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.Version]{}, err
	}
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.Version]{}, err
	}
	rows, err := reader.statements.ListEntityHistory(ctx, db.ListEntityHistoryParams{
		ZoneID:       reader.statements.zoneID,
		EntityID:     string(entityID),
		AfterVersion: int64(after),
		PageSize:     pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.Version]{}, classifySQLite("list Entity history", err)
	}
	items := make([]knowledge.KnowledgeVersion[knowledge.Entity], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		version, found, err := restoreEntity(ctx, reader.statements, entityVersionRecord{
			id:          entityID,
			title:       row.Title,
			entityType:  row.EntityType,
			version:     row.Version,
			deleted:     row.Deleted,
			hash:        row.ContentHash,
			description: row.Description,
		}, nil)
		if err != nil || !found {
			return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.Version]{}, storedVersionError("Entity", string(entityID), err)
		}
		items = append(items, version)
	}
	return entityHistoryPage(items, len(rows) > limit), nil
}

func (reader relationReader) History(
	ctx context.Context,
	relationID knowledge.RelationID,
	after knowledge.Version,
	limit int,
) (knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.Version], error) {
	if err := knowledge.ValidateRelationID(relationID); err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.Version]{}, err
	}
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.Version]{}, err
	}
	rows, err := reader.statements.ListRelationHistory(ctx, db.ListRelationHistoryParams{
		ZoneID:       reader.statements.zoneID,
		RelationID:   string(relationID),
		AfterVersion: int64(after),
		PageSize:     pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.Version]{}, classifySQLite("list Relation history", err)
	}
	items := make([]knowledge.KnowledgeVersion[knowledge.Relation], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		version, found, err := restoreRelation(relationVersionRecord{
			id:             relationID,
			sourceEntityID: row.SourceEntityID,
			targetEntityID: row.TargetEntityID,
			relationType:   row.RelationType,
			version:        row.Version,
			deleted:        row.Deleted,
			hash:           row.ContentHash,
			description:    row.Description,
		}, nil)
		if err != nil || !found {
			return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.Version]{}, storedVersionError("Relation", string(relationID), err)
		}
		items = append(items, version)
	}
	return relationHistoryPage(items, len(rows) > limit), nil
}

func (reader claimReader) History(
	ctx context.Context,
	claimID knowledge.ClaimID,
	after knowledge.Version,
	limit int,
) (knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.Version], error) {
	if err := knowledge.ValidateClaimID(claimID); err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.Version]{}, err
	}
	pageSize, err := readPageSize(limit)
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.Version]{}, err
	}
	rows, err := reader.statements.ListClaimHistory(ctx, db.ListClaimHistoryParams{
		ZoneID:       reader.statements.zoneID,
		ClaimID:      string(claimID),
		AfterVersion: int64(after),
		PageSize:     pageSize,
	})
	if err != nil {
		return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.Version]{}, classifySQLite("list Claim history", err)
	}
	items := make([]knowledge.KnowledgeVersion[knowledge.Claim], 0, min(limit, len(rows)))
	for _, row := range rows[:min(limit, len(rows))] {
		version, found, err := restoreClaim(claimVersionRecord{
			id:          claimID,
			subjectKind: row.SubjectKind,
			subjectID:   row.SubjectID,
			claimType:   row.ClaimType,
			version:     row.Version,
			deleted:     row.Deleted,
			hash:        row.ContentHash,
			description: row.Description,
		}, nil)
		if err != nil || !found {
			return knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.Version]{}, storedVersionError("Claim", string(claimID), err)
		}
		items = append(items, version)
	}
	return claimHistoryPage(items, len(rows) > limit), nil
}

func (reader entityReader) Read(
	ctx context.Context,
	references []knowledge.Reference[knowledge.EntityID],
) ([]knowledge.KnowledgeVersion[knowledge.Entity], error) {
	result := make([]knowledge.KnowledgeVersion[knowledge.Entity], 0, len(references))
	for _, reference := range references {
		if err := validateReference(reference.ID, reference.Version); err != nil {
			return nil, err
		}
		row, err := reader.statements.GetEntityVersion(ctx, db.GetEntityVersionParams{
			ZoneID:   reader.statements.zoneID,
			EntityID: string(reference.ID),
			Version:  int64(reference.Version),
		})
		version, found, err := restoreEntity(ctx, reader.statements, entityVersionRecord{
			id:          reference.ID,
			title:       row.Title,
			entityType:  row.EntityType,
			version:     row.Version,
			deleted:     row.Deleted,
			hash:        row.ContentHash,
			description: row.Description,
		}, err)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("%w: Entity %q Version %d", knowledge.ErrNotFound, reference.ID, reference.Version)
		}
		result = append(result, version)
	}
	return result, nil
}

func (reader relationReader) Read(
	ctx context.Context,
	references []knowledge.Reference[knowledge.RelationID],
) ([]knowledge.KnowledgeVersion[knowledge.Relation], error) {
	result := make([]knowledge.KnowledgeVersion[knowledge.Relation], 0, len(references))
	for _, reference := range references {
		if err := validateReference(reference.ID, reference.Version); err != nil {
			return nil, err
		}
		row, err := reader.statements.GetRelationVersion(ctx, db.GetRelationVersionParams{
			ZoneID:     reader.statements.zoneID,
			RelationID: string(reference.ID),
			Version:    int64(reference.Version),
		})
		version, found, err := restoreRelation(relationVersionRecord{
			id:             reference.ID,
			sourceEntityID: row.SourceEntityID,
			targetEntityID: row.TargetEntityID,
			relationType:   row.RelationType,
			version:        row.Version,
			deleted:        row.Deleted,
			hash:           row.ContentHash,
			description:    row.Description,
		}, err)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("%w: Relation %q Version %d", knowledge.ErrNotFound, reference.ID, reference.Version)
		}
		result = append(result, version)
	}
	return result, nil
}

func (reader claimReader) Read(
	ctx context.Context,
	references []knowledge.Reference[knowledge.ClaimID],
) ([]knowledge.KnowledgeVersion[knowledge.Claim], error) {
	result := make([]knowledge.KnowledgeVersion[knowledge.Claim], 0, len(references))
	for _, reference := range references {
		if err := validateReference(reference.ID, reference.Version); err != nil {
			return nil, err
		}
		row, err := reader.statements.GetClaimVersion(ctx, db.GetClaimVersionParams{
			ZoneID:  reader.statements.zoneID,
			ClaimID: string(reference.ID),
			Version: int64(reference.Version),
		})
		version, found, err := restoreClaim(claimVersionRecord{
			id:          reference.ID,
			subjectKind: row.SubjectKind,
			subjectID:   row.SubjectID,
			claimType:   row.ClaimType,
			version:     row.Version,
			deleted:     row.Deleted,
			hash:        row.ContentHash,
			description: row.Description,
		}, err)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("%w: Claim %q Version %d", knowledge.ErrNotFound, reference.ID, reference.Version)
		}
		result = append(result, version)
	}
	return result, nil
}

func readPageSize(limit int) (int64, error) {
	if limit <= 0 || uint64(limit) >= uint64(math.MaxInt64) {
		return 0, fmt.Errorf("%w: Knowledge page limit must be positive and representable", knowledge.ErrInvalidChange)
	}
	return int64(limit + 1), nil
}

func validateReference(id knowledge.ObjectRef, version knowledge.Version) error {
	if id == nil || id.ObjectID() == "" || version == 0 {
		return fmt.Errorf("%w: Knowledge Reference is invalid", knowledge.ErrInvalidChange)
	}
	return knowledge.ValidateVersion(version)
}

func storedVersionError(kind string, id string, err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: listed %s %q cannot be restored", knowledge.ErrDataIntegrity, kind, id)
}

func entityReferencePage(
	items []knowledge.Reference[knowledge.EntityID],
	hasMore bool,
) knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID] {
	page := knowledge.Page[knowledge.Reference[knowledge.EntityID], knowledge.EntityID]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].ID
	}
	return page
}

func relationReferencePage(
	items []knowledge.Reference[knowledge.RelationID],
	hasMore bool,
) knowledge.Page[knowledge.Reference[knowledge.RelationID], knowledge.RelationID] {
	page := knowledge.Page[knowledge.Reference[knowledge.RelationID], knowledge.RelationID]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].ID
	}
	return page
}

func claimReferencePage(
	items []knowledge.Reference[knowledge.ClaimID],
	hasMore bool,
) knowledge.Page[knowledge.Reference[knowledge.ClaimID], knowledge.ClaimID] {
	page := knowledge.Page[knowledge.Reference[knowledge.ClaimID], knowledge.ClaimID]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].ID
	}
	return page
}

func entityVersionPage(
	items []knowledge.KnowledgeVersion[knowledge.Entity],
	hasMore bool,
) knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.EntityID] {
	page := knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.EntityID]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].Knowledge.ID
	}
	return page
}

func relationVersionPage(
	items []knowledge.KnowledgeVersion[knowledge.Relation],
	hasMore bool,
) knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.RelationID] {
	page := knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.RelationID]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].Knowledge.ID
	}
	return page
}

func claimVersionPage(
	items []knowledge.KnowledgeVersion[knowledge.Claim],
	hasMore bool,
) knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.ClaimID] {
	page := knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.ClaimID]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].Knowledge.ID
	}
	return page
}

func entityHistoryPage(
	items []knowledge.KnowledgeVersion[knowledge.Entity],
	hasMore bool,
) knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.Version] {
	page := knowledge.Page[knowledge.KnowledgeVersion[knowledge.Entity], knowledge.Version]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].Version
	}
	return page
}

func relationHistoryPage(
	items []knowledge.KnowledgeVersion[knowledge.Relation],
	hasMore bool,
) knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.Version] {
	page := knowledge.Page[knowledge.KnowledgeVersion[knowledge.Relation], knowledge.Version]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].Version
	}
	return page
}

func claimHistoryPage(
	items []knowledge.KnowledgeVersion[knowledge.Claim],
	hasMore bool,
) knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.Version] {
	page := knowledge.Page[knowledge.KnowledgeVersion[knowledge.Claim], knowledge.Version]{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		page.NextAfter = items[len(items)-1].Version
	}
	return page
}
