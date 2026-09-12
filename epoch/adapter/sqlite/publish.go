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

// Publish compares Current with PublicationTarget.ExpectedEpoch, rejects a regressing
// Knowledge Versions, then creates and selects one immutable Epoch in the same immediate
// transaction. It never invokes cross-context readiness checks.
func (s *Store) Publish(
	ctx context.Context,
	target epoch.PublicationTarget,
	corporaID epoch.CorporaID,
	publishedAt time.Time,
) (epoch.Epoch, error) {
	if err := epoch.ValidatePublication(target, corporaID, publishedAt); err != nil {
		return epoch.Epoch{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return epoch.Epoch{}, err
	}
	var published epoch.Epoch
	work := func(statements statements) error {
		currentID, err := statements.GetCurrentEpochID(ctx, statements.zoneID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			count, countErr := statements.CountEpochs(ctx, statements.zoneID)
			if countErr != nil {
				return classifySQLite("count Epoch rows before initial publication", countErr)
			}
			if count != 0 {
				return fmt.Errorf(
					"%w: %d Epoch rows exist without Current selection",
					epoch.ErrEpochDataIntegrity,
					count,
				)
			}
			if target.ExpectedEpoch != 0 {
				return fmt.Errorf(
					"%w: expected %d but no Current Epoch exists",
					epoch.ErrEpochConflict,
					target.ExpectedEpoch,
				)
			}
		case err != nil:
			return classifySQLite("read Current Epoch before publication", err)
		default:
			currentRow, readErr := statements.GetEpoch(ctx, db.GetEpochParams{
				ZoneID: statements.zoneID, ID: currentID,
			})
			if errors.Is(readErr, sql.ErrNoRows) {
				return fmt.Errorf(
					"%w: Current references missing Epoch %d",
					epoch.ErrEpochDataIntegrity,
					currentID,
				)
			}
			if readErr != nil {
				return classifySQLite("read Current Epoch before publication", readErr)
			}
			current, decodeErr := decodeEpoch(ctx, statements, currentRow)
			if decodeErr != nil {
				return decodeErr
			}
			if current.ID != target.ExpectedEpoch {
				return fmt.Errorf(
					"%w: expected %d but Current is %d",
					epoch.ErrEpochConflict,
					target.ExpectedEpoch,
					current.ID,
				)
			}
		}

		digest, err := target.Knowledge.Digest()
		if err != nil {
			return err
		}
		id, err := statements.CreateEpoch(ctx, db.CreateEpochParams{
			ZoneID:          statements.zoneID,
			KnowledgeDigest: digest,
			CorporaID:       string(corporaID),
			StructureID:     string(target.StructureID),
			PublishedAt:     publishedAt.Format(time.RFC3339Nano),
		})
		if err != nil {
			return classifySQLite("create Epoch", err)
		}
		if id <= int64(target.ExpectedEpoch) {
			return fmt.Errorf(
				"%w: allocated Epoch %d does not follow Current %d",
				epoch.ErrEpochDataIntegrity,
				id,
				target.ExpectedEpoch,
			)
		}
		if target.ExpectedEpoch == 0 && id != 1 {
			return fmt.Errorf(
				"%w: initial Epoch ID is %d; expected 1",
				epoch.ErrEpochDataIntegrity,
				id,
			)
		}
		published = epoch.Epoch{
			ID:          epoch.ID(id),
			Knowledge:   target.Knowledge.Clone(),
			CorporaID:   corporaID,
			StructureID: target.StructureID,
			PublishedAt: publishedAt,
		}
		if err := epoch.ValidateEpoch(published); err != nil {
			return err
		}
		if err := saveEpochVersions(ctx, statements, id, target.Knowledge); err != nil {
			return err
		}
		if err := statements.SelectCurrentEpoch(ctx, db.SelectCurrentEpochParams{
			ZoneID: statements.zoneID, EpochID: id,
		}); err != nil {
			return classifySQLite("select Current Epoch", err)
		}
		return nil
	}
	executor, err := transactionsqlite.Current(ctx, s.database)
	switch {
	case err == nil:
		err = work(newStatements(executor, string(zoneID)))
	case errors.Is(err, transactionsqlite.ErrNoTransaction):
		err = s.write(ctx, work)
	default:
		return epoch.Epoch{}, fmt.Errorf("join Epoch publication transaction: %w", err)
	}
	if err != nil {
		return epoch.Epoch{}, err
	}
	return published, nil
}

func saveEpochVersions(
	ctx context.Context,
	statements statements,
	epochID int64,
	versions knowledge.Manifest,
) error {
	for _, reference := range versions.Entities {
		if err := statements.AddEpochEntityVersion(ctx, db.AddEpochEntityVersionParams{
			ZoneID:   statements.zoneID,
			EpochID:  epochID,
			EntityID: string(reference.ID),
			Version:  int64(reference.Version),
		}); err != nil {
			return classifySQLite("save Epoch Entity Version", err)
		}
	}
	for _, reference := range versions.Relations {
		if err := statements.AddEpochRelationVersion(ctx, db.AddEpochRelationVersionParams{
			ZoneID:     statements.zoneID,
			EpochID:    epochID,
			RelationID: string(reference.ID),
			Version:    int64(reference.Version),
		}); err != nil {
			return classifySQLite("save Epoch Relation Version", err)
		}
	}
	for _, reference := range versions.Claims {
		if err := statements.AddEpochClaimVersion(ctx, db.AddEpochClaimVersionParams{
			ZoneID:  statements.zoneID,
			EpochID: epochID,
			ClaimID: string(reference.ID),
			Version: int64(reference.Version),
		}); err != nil {
			return classifySQLite("save Epoch Claim Version", err)
		}
	}
	return nil
}
