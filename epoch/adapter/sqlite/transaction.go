package sqlite

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/zone"
)

func (s *Store) write(
	ctx context.Context,
	work func(statements) error,
) (resultErr error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	connection, err := s.database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Epoch write connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("release Epoch write connection", closeErr),
			)
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Epoch write", err)
	}
	open := true
	defer func() {
		if !open {
			return
		}
		if _, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK"); rollbackErr != nil {
			resultErr = errors.Join(
				resultErr,
				classifySQLite("roll back Epoch write", rollbackErr),
			)
		}
	}()
	if err := work(newStatements(connection, string(zoneID))); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Epoch write", err)
	}
	open = false
	return nil
}
