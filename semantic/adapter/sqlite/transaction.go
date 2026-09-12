package sqlite

import (
	"context"
	"errors"
)

func (s *Store) write(
	ctx context.Context,
	work func(statements, sqlExecutor) error,
) (resultErr error) {
	connection, err := s.database.Conn(ctx)
	if err != nil {
		return classifySQLite("reserve Semantic SQLite write connection", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, classifySQLite(
				"release Semantic SQLite write connection",
				closeErr,
			))
		}
	}()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return classifySQLite("begin Semantic SQLite write", err)
	}
	open := true
	defer func() {
		if open {
			_, rollbackErr := connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
			resultErr = errors.Join(resultErr, classifySQLite(
				"roll back Semantic SQLite write",
				rollbackErr,
			))
		}
	}()
	if err := work(newStatements(connection), connection); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return classifySQLite("commit Semantic SQLite write", err)
	}
	open = false
	return nil
}
