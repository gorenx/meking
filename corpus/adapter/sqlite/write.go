package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/textunits"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

func saveTextUnit(
	ctx context.Context,
	statements statements,
	zoneID string,
	unit textunits.TextUnitBody,
) error {
	if err := textunits.ValidateTextUnitBody(unit); err != nil {
		return fmt.Errorf("%w: %v", corpus.ErrInvalidCorpus, err)
	}
	inserted, err := statements.SaveTextUnit(ctx, db.SaveTextUnitParams{
		ZoneID: zoneID, ID: string(unit.ID), Text: unit.Text,
	})
	if err != nil {
		return classifySQLite("save TextUnit", err)
	}
	if inserted == 1 {
		return nil
	}
	existing, err := statements.GetTextUnit(ctx, db.GetTextUnitParams{
		ZoneID: zoneID, ID: string(unit.ID),
	})
	if err != nil {
		return classifySQLite("read conflicting TextUnit", err)
	}
	if existing.Text != unit.Text {
		return corpus.ErrContentConflict
	}
	return nil
}

func (s *Store) Activate(ctx context.Context, id corpus.CorporaID) error {
	if err := corpus.ValidateCorporaID(id); err != nil {
		return err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	zoneIDValue := string(zoneID)
	return s.write(ctx, zoneIDValue, func(statements statements) error {
		state, err := statements.GetCorpusState(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("read Corpus state for activation", err)
		}
		if state.CurrentCorporaID == corporaIDSequence(id) {
			return nil
		}
		if !state.BuildingCorporaID.Valid || state.BuildingCorporaID.Int64 != corporaIDSequence(id) ||
			state.BuildingState.String != "prepared" {
			return corpus.ErrCorporaActivationConflict
		}
		updated, err := statements.ActivateCorpus(ctx, db.ActivateCorpusParams{
			ZoneID:            zoneIDValue,
			BuildingCorporaID: sql.NullInt64{Int64: corporaIDSequence(id), Valid: true},
		})
		if err != nil {
			return classifySQLite("activate Corpus", err)
		}
		if updated != 1 {
			return corpus.ErrCorporaActivationConflict
		}
		if err := statements.ClearTextChunkingProgress(ctx, zoneIDValue); err != nil {
			return classifySQLite("clear activated Corpus chunking progress", err)
		}
		return nil
	})
}

func (s *Store) write(
	ctx context.Context,
	zoneID string,
	work func(statements) error,
) (resultErr error) {
	if executor, err := transactionsqlite.Current(ctx, s.database); err == nil {
		return work(newStatements(executor))
	} else if !errors.Is(err, transactionsqlite.ErrNoTransaction) {
		return classifySQLite("join Corpus write transaction", err)
	}
	connection, err := s.database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Corpus write connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, classifySQLite("release Corpus write connection", closeErr))
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Corpus write", err)
	}
	open := true
	defer func() {
		if open {
			_, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
			if rollbackErr != nil {
				resultErr = errors.Join(resultErr, classifySQLite("roll back Corpus write", rollbackErr))
			}
		}
	}()
	if err := work(newStatements(connection)); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Corpus write", err)
	}
	open = false
	return nil
}

func (s *Store) reader(ctx context.Context) (statements, string, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, "", err
	}
	executor, err := transactionsqlite.Current(ctx, s.database)
	if err == nil {
		return newStatements(executor), string(zoneID), nil
	}
	if errors.Is(err, transactionsqlite.ErrNoTransaction) {
		return newStatements(s.database), string(zoneID), nil
	}
	return nil, "", classifySQLite("resolve Corpus read transaction", err)
}
