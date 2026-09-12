package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/epoch"
	"github.com/memoria-space/meking/epoch/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/knowledge"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

// Current reads the selected Epoch ID and immutable row in one deferred
// transaction so a concurrent publication cannot mix two selections.
func (s *Store) Current(ctx context.Context) (epoch.Epoch, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return epoch.Epoch{}, err
	}
	executor, transactionErr := transactionsqlite.Current(ctx, s.database)
	switch {
	case transactionErr == nil:
		return readCurrentEpoch(ctx, newStatements(executor, string(zoneID)))
	case !errors.Is(transactionErr, transactionsqlite.ErrNoTransaction):
		return epoch.Epoch{}, fmt.Errorf("join Current Epoch read transaction: %w", transactionErr)
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return epoch.Epoch{}, classifySQLite("begin Current Epoch read", err)
	}
	defer transaction.Rollback()
	return readCurrentEpoch(ctx, newStatements(transaction, string(zoneID)))
}

func readCurrentEpoch(ctx context.Context, statements statements) (epoch.Epoch, error) {
	id, err := statements.GetCurrentEpochID(ctx, statements.zoneID)
	if errors.Is(err, sql.ErrNoRows) {
		count, countErr := statements.CountEpochs(ctx, statements.zoneID)
		if countErr != nil {
			return epoch.Epoch{}, classifySQLite("count Epoch rows without Current", countErr)
		}
		if count != 0 {
			return epoch.Epoch{}, fmt.Errorf(
				"%w: %d Epoch rows exist without Current selection",
				epoch.ErrEpochDataIntegrity,
				count,
			)
		}
		return epoch.Epoch{}, epoch.ErrEpochNotFound
	}
	if err != nil {
		return epoch.Epoch{}, classifySQLite("read Current Epoch ID", err)
	}
	row, err := statements.GetEpoch(ctx, db.GetEpochParams{ZoneID: statements.zoneID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return epoch.Epoch{}, fmt.Errorf(
			"%w: Current references missing Epoch %d",
			epoch.ErrEpochDataIntegrity,
			id,
		)
	}
	if err != nil {
		return epoch.Epoch{}, classifySQLite("read Current Epoch", err)
	}
	return decodeEpoch(ctx, statements, row)
}

// Load returns one exact immutable Epoch and never consults Current.
func (s *Store) Load(ctx context.Context, id epoch.ID) (epoch.Epoch, error) {
	if id <= 0 {
		return epoch.Epoch{}, fmt.Errorf("%w: requested ID must be positive", epoch.ErrInvalidEpoch)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return epoch.Epoch{}, err
	}
	statements := newStatements(s.database, string(zoneID))
	row, err := statements.GetEpoch(ctx, db.GetEpochParams{
		ZoneID: string(zoneID), ID: int64(id),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return epoch.Epoch{}, epoch.ErrEpochNotFound
	}
	if err != nil {
		return epoch.Epoch{}, classifySQLite("read Epoch", err)
	}
	return decodeEpoch(ctx, statements, row)
}

func (s *Store) LoadStructure(
	ctx context.Context,
	id epoch.StructureID,
) (epoch.Epoch, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return epoch.Epoch{}, err
	}
	statements := newStatements(s.database, string(zoneID))
	row, err := statements.GetEpochByStructure(
		ctx,
		db.GetEpochByStructureParams{
			ZoneID:      string(zoneID),
			StructureID: string(id),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return epoch.Epoch{}, epoch.ErrEpochNotFound
	}
	if err != nil {
		return epoch.Epoch{}, classifySQLite("read Epoch by Structure", err)
	}
	return decodeEpoch(ctx, statements, row)
}

func decodeEpoch(ctx context.Context, statements statements, row db.Epoch) (epoch.Epoch, error) {
	publishedAt, err := time.Parse(time.RFC3339Nano, row.PublishedAt)
	if err != nil {
		return epoch.Epoch{}, fmt.Errorf(
			"%w: Epoch %d has invalid PublishedAt: %v",
			epoch.ErrEpochDataIntegrity,
			row.ID,
			err,
		)
	}
	versions, err := loadEpochVersions(ctx, statements, row.ID)
	if err != nil {
		return epoch.Epoch{}, err
	}
	digest, err := versions.Digest()
	if err != nil || digest != row.KnowledgeDigest {
		return epoch.Epoch{}, epoch.ErrEpochDataIntegrity
	}
	value := epoch.Epoch{
		ID:          epoch.ID(row.ID),
		Knowledge:   versions,
		CorporaID:   epoch.CorporaID(row.CorporaID),
		StructureID: epoch.StructureID(row.StructureID),
		PublishedAt: publishedAt,
	}
	if err := epoch.ValidateEpoch(value); err != nil {
		return epoch.Epoch{}, fmt.Errorf(
			"%w: stored Epoch %d is invalid: %v",
			epoch.ErrEpochDataIntegrity,
			row.ID,
			err,
		)
	}
	return value, nil
}

func loadEpochVersions(
	ctx context.Context,
	statements statements,
	epochID int64,
) (knowledge.Manifest, error) {
	entities, err := statements.ListEpochEntityVersions(ctx, db.ListEpochEntityVersionsParams{
		ZoneID:  statements.zoneID,
		EpochID: epochID,
	})
	if err != nil {
		return knowledge.Manifest{}, classifySQLite("read Epoch Entity Versions", err)
	}
	relations, err := statements.ListEpochRelationVersions(ctx, db.ListEpochRelationVersionsParams{
		ZoneID:  statements.zoneID,
		EpochID: epochID,
	})
	if err != nil {
		return knowledge.Manifest{}, classifySQLite("read Epoch Relation Versions", err)
	}
	claims, err := statements.ListEpochClaimVersions(ctx, db.ListEpochClaimVersionsParams{
		ZoneID:  statements.zoneID,
		EpochID: epochID,
	})
	if err != nil {
		return knowledge.Manifest{}, classifySQLite("read Epoch Claim Versions", err)
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
		return knowledge.Manifest{}, epoch.ErrEpochDataIntegrity
	}
	return result, nil
}
