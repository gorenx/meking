package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community/adapter/sqlite/internal/db"

	"github.com/memoria-space/meking/community"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

// Save writes one complete immutable CommunitySet in a single immediate
// transaction. It does not select a current ReportSet or execute derived work.
func (s *Store) Save(ctx context.Context, set community.CommunitySet) error {
	if err := community.ValidateCommunitySet(set); err != nil {
		return err
	}
	return s.write(ctx, func(statements statements) error {
		if _, err := statements.GetCommunitySet(ctx, string(set.ID)); err == nil {
			return fmt.Errorf(
				"%w: CommunitySet ID %q already exists",
				community.ErrCommunitySetConflict,
				set.ID,
			)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return classifySQLite("read existing CommunitySet", err)
		}
		if err := statements.CreateCommunitySet(ctx, db.CreateCommunitySetParams{
			ID:              string(set.ID),
			DetectorVersion: int64(set.DetectorVersion),
			MaxClusterSize:  int64(set.DetectionConfig.MaxClusterSize),
			UseLargestConnectedComponent: boolInteger(
				set.DetectionConfig.UseLargestConnectedComponent,
			),
			Seed:      set.DetectionConfig.Seed,
			CreatedAt: set.CreatedAt.Format(communitySetTimeFormat),
		}); err != nil {
			return classifySQLite("create CommunitySet", err)
		}
		for _, entity := range set.Entities {
			if err := statements.AddCommunityEntity(ctx, db.AddCommunityEntityParams{
				CommunitySetID: string(set.ID),
				EntityID:       entity.ID,
				Version:        int64(entity.Version),
			}); err != nil {
				return classifySQLite("add CommunitySet Entity reference", err)
			}
		}
		for _, relation := range set.Relations {
			if err := statements.AddCommunityRelation(ctx, db.AddCommunityRelationParams{
				CommunitySetID: string(set.ID),
				RelationID:     relation.ID,
				Version:        int64(relation.Version),
			}); err != nil {
				return classifySQLite("add CommunitySet Relation reference", err)
			}
		}
		for _, current := range set.Communities {
			parentID := sql.NullString{}
			if current.ParentID != nil {
				parentID = sql.NullString{String: string(*current.ParentID), Valid: true}
			}
			if err := statements.AddCommunity(ctx, db.AddCommunityParams{
				CommunitySetID:    string(set.ID),
				CommunityID:       string(current.ID),
				Number:            int64(current.Number),
				Level:             int64(current.Level),
				ParentCommunityID: parentID,
				Final:             boolInteger(current.Final),
				Unsplittable:      boolInteger(current.Unsplittable),
			}); err != nil {
				return classifySQLite("add CommunitySet Community", err)
			}
			for ordinal, entityID := range current.EntityIDs {
				if err := statements.AddCommunityMember(
					ctx,
					db.AddCommunityMemberParams{
						CommunitySetID: string(set.ID),
						CommunityID:    string(current.ID),
						MemberOrdinal:  int64(ordinal),
						EntityID:       entityID,
					},
				); err != nil {
					return classifySQLite("add Community member", err)
				}
			}
		}
		return nil
	})
}

func (s *Store) write(
	ctx context.Context,
	work func(statements) error,
) (resultErr error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return fmt.Errorf("write Community data: %w", err)
	}
	if executor, err := transactionsqlite.Current(ctx, s.database); err == nil {
		return work(newStatements(executor, string(zoneID)))
	} else if !errors.Is(err, transactionsqlite.ErrNoTransaction) {
		return classifySQLite("join Community write transaction", err)
	}
	connection, err := s.database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Community write connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("release Community write connection", closeErr),
			)
		}
	}()

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Community write transaction", err)
	}
	transactionOpen := true
	defer func() {
		if !transactionOpen {
			return
		}
		_, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		if rollbackErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("roll back Community write transaction", rollbackErr),
			)
		}
	}()

	if err := work(newStatements(connection, string(zoneID))); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Community write transaction", err)
	}
	transactionOpen = false
	return nil
}

func boolInteger(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
