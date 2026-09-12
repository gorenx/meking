package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/memoria-space/meking/semantic"
	"github.com/memoria-space/meking/semantic/adapter/sqlite/internal/db"
)

type encodedVector struct {
	id        string
	embedding string
}

// Add atomically retains one batch of vectors in a Namespace. Repeating the
// same batch is idempotent; vectors already retained under the same opaque ID
// remain the canonical cached representation.
func (s *Store) Add(
	ctx context.Context,
	namespace semantic.Namespace,
	vectors []semantic.Vector,
) error {
	if err := semantic.ValidateNamespace(namespace); err != nil {
		return err
	}
	zoneID, err := semanticZoneID(ctx)
	if err != nil {
		return err
	}
	prepared, dimension, err := prepareVectors(vectors)
	if err != nil {
		return err
	}
	var added, total int64
	err = s.write(ctx, func(statements statements, executor sqlExecutor) error {
		configured, err := fixDimension(ctx, statements, dimension)
		if err != nil {
			return err
		}
		if err = statements.RetainNamespace(ctx, db.RetainNamespaceParams{
			ZoneID: zoneID, Name: string(namespace),
		}); err != nil {
			return classifySQLite("retain Semantic Namespace", err)
		}
		storedNamespace, err := statements.GetNamespace(ctx, db.GetNamespaceParams{
			ZoneID: zoneID, Name: string(namespace),
		})
		if err != nil {
			return classifySQLite("read retained Semantic Namespace", err)
		}
		table := ""
		if len(prepared) > 0 {
			table, err = ensureVectorTable(ctx, executor, int(configured.Dimension))
			if err != nil {
				return err
			}
		}
		for _, vector := range prepared {
			retained, err := retainVector(
				ctx, statements, executor, table, zoneID, storedNamespace.ID, vector,
			)
			if err != nil {
				return err
			}
			if retained {
				added++
			}
		}
		total, err = statements.CountNamespaceVectors(ctx, db.CountNamespaceVectorsParams{
			ZoneID: zoneID, NamespaceRowID: storedNamespace.ID,
		})
		if err != nil {
			return classifySQLite("count Semantic Namespace vectors", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.logger.LogAttrs(
		ctx, slog.LevelInfo, "Semantic vectors added",
		slog.String("zone_id", zoneID),
		slog.String("namespace", string(namespace)),
		slog.Int64("added_vectors", added), slog.Int64("total_vectors", total),
	)
	return nil
}

func prepareVectors(vectors []semantic.Vector) ([]encodedVector, int, error) {
	prepared := make([]encodedVector, len(vectors))
	dimension := 0
	for index, vector := range vectors {
		if err := semantic.ValidateID(vector.ID, "vector ID"); err != nil {
			return nil, 0, err
		}
		normalized, err := semantic.NormalizeVector(vector.Values)
		if err != nil {
			return nil, 0, fmt.Errorf("validate Semantic vector %q: %w", vector.ID, err)
		}
		if dimension == 0 {
			dimension = len(normalized)
		}
		if len(normalized) != dimension {
			return nil, 0, fmt.Errorf(
				"Semantic vector %q dimension %d does not match %d",
				vector.ID, len(normalized), dimension,
			)
		}
		encoded, err := json.Marshal(normalized)
		if err != nil {
			return nil, 0, fmt.Errorf("encode Semantic vector %q: %w", vector.ID, err)
		}
		prepared[index] = encodedVector{id: vector.ID, embedding: string(encoded)}
	}
	return prepared, dimension, nil
}

func retainVector(
	ctx context.Context,
	statements statements,
	executor sqlExecutor,
	table string,
	zoneID string,
	namespaceRowID int64,
	vector encodedVector,
) (bool, error) {
	if err := statements.RetainVector(ctx, db.RetainVectorParams{
		VectorID: vector.id, Embedding: vector.embedding,
	}); err != nil {
		return false, classifySQLite("retain Semantic vector", err)
	}
	stored, err := statements.GetVector(ctx, vector.id)
	if err != nil {
		return false, classifySQLite("read retained Semantic vector", err)
	}
	memberID, err := statements.RetainNamespaceVector(ctx, db.RetainNamespaceVectorParams{
		ZoneID: zoneID, NamespaceRowID: namespaceRowID, VectorRowID: stored.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, classifySQLite("add Semantic Namespace vector", err)
	}
	if err = insertVector(
		ctx, executor, table, memberID, namespaceRowID, vector.id, stored.Embedding,
	); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) Delete(ctx context.Context, namespace semantic.Namespace) error {
	if err := semantic.ValidateNamespace(namespace); err != nil {
		return err
	}
	zoneID, err := semanticZoneID(ctx)
	if err != nil {
		return err
	}
	return s.write(ctx, func(statements statements, executor sqlExecutor) error {
		stored, err := statements.GetNamespace(ctx, db.GetNamespaceParams{
			ZoneID: zoneID, Name: string(namespace),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return classifySQLite("read Semantic Namespace for deletion", err)
		}
		configured, err := vectorDatabase(ctx, statements)
		if err != nil {
			return err
		}
		if configured.Dimension > 0 {
			table, err := existingVectorTable(ctx, executor, int(configured.Dimension))
			if err != nil {
				return err
			}
			if err = deleteVectors(ctx, executor, table, stored.ID); err != nil {
				return err
			}
		}
		if err = statements.DeleteNamespaceVectors(ctx, db.DeleteNamespaceVectorsParams{
			ZoneID: zoneID, NamespaceRowID: stored.ID,
		}); err != nil {
			return classifySQLite("delete Semantic Namespace membership", err)
		}
		if err = statements.DeleteNamespace(ctx, db.DeleteNamespaceParams{ZoneID: zoneID, ID: stored.ID}); err != nil {
			return classifySQLite("delete Semantic Namespace", err)
		}
		return nil
	})
}
