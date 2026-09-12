package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ensureVectorTable(ctx context.Context, executor sqlExecutor, dimension int) (string, error) {
	name, ddl, err := vectorTableDefinition(dimension)
	if err != nil {
		return "", err
	}
	var stored string
	err = executor.QueryRowContext(
		ctx, `SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = ?`, name,
	).Scan(&stored)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := executor.ExecContext(ctx, ddl); err != nil {
			return "", classifySQLite("create Semantic vec0 table", err)
		}
	case err != nil:
		return "", classifySQLite("inspect Semantic vec0 table", err)
	case stored != ddl:
		return "", fmt.Errorf("Semantic vec0 table %q has an unexpected definition", name)
	}
	return name, nil
}

func existingVectorTable(ctx context.Context, executor sqlExecutor, dimension int) (string, error) {
	name, ddl, err := vectorTableDefinition(dimension)
	if err != nil {
		return "", err
	}
	var stored string
	if err := executor.QueryRowContext(
		ctx, `SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = ?`, name,
	).Scan(&stored); err != nil {
		return "", classifySQLite("read Semantic vec0 table", err)
	}
	if stored != ddl {
		return "", fmt.Errorf("Semantic vec0 table %q has an unexpected definition", name)
	}
	return name, nil
}

func vectorTableDefinition(dimension int) (string, string, error) {
	if dimension <= 0 {
		return "", "", errors.New("Semantic vec0 dimension must be positive")
	}
	name := fmt.Sprintf("vector_values_%d", dimension)
	return name, fmt.Sprintf(
		`CREATE VIRTUAL TABLE %q USING vec0(embedding float[%d] distance_metric=cosine, namespace_row_id integer partition key, vector_id text)`,
		name, dimension,
	), nil
}

func insertVector(
	ctx context.Context,
	executor sqlExecutor,
	table string,
	recordID int64,
	namespaceRowID int64,
	vectorID string,
	vectorJSON string,
) error {
	statement := fmt.Sprintf(
		`INSERT INTO %q (rowid, embedding, namespace_row_id, vector_id) VALUES (?, vec_f32(?), ?, ?)`,
		table,
	)
	if _, err := executor.ExecContext(
		ctx, statement, recordID, vectorJSON, namespaceRowID, vectorID,
	); err != nil {
		return classifySQLite("insert Semantic vec0 row", err)
	}
	return nil
}

func deleteVectors(ctx context.Context, executor sqlExecutor, table string, namespaceRowID int64) error {
	statement := fmt.Sprintf(`DELETE FROM %q WHERE namespace_row_id = ?`, table)
	if _, err := executor.ExecContext(ctx, statement, namespaceRowID); err != nil {
		return classifySQLite("delete Semantic vec0 rows", err)
	}
	return nil
}

func countVectors(
	ctx context.Context,
	executor sqlExecutor,
	table string,
	namespaceRowID int64,
) (int64, error) {
	statement := fmt.Sprintf(`SELECT count(*) FROM %q WHERE namespace_row_id = ?`, table)
	var count int64
	if err := executor.QueryRowContext(ctx, statement, namespaceRowID).Scan(&count); err != nil {
		return 0, classifySQLite("count Semantic vec0 rows", err)
	}
	return count, nil
}

func countInvalidNamespaceVectors(
	ctx context.Context,
	executor sqlExecutor,
	table string,
	namespaceRowID int64,
) (int64, error) {
	statement := fmt.Sprintf(`
		SELECT count(*)
		FROM namespace_vectors AS member
		LEFT JOIN vectors AS vector ON vector.id = member.vector_row_id
		LEFT JOIN %q AS indexed ON indexed.rowid = member.id
		WHERE member.namespace_row_id = ?
		  AND (
			vector.id IS NULL OR
			indexed.rowid IS NULL OR
			indexed.namespace_row_id != member.namespace_row_id OR
			indexed.vector_id != vector.vector_id
		  )
	`, table)
	var count int64
	if err := executor.QueryRowContext(ctx, statement, namespaceRowID).Scan(&count); err != nil {
		return 0, classifySQLite("validate Semantic Namespace vector references", err)
	}
	return count, nil
}
