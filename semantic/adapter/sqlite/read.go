package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/memoria-space/meking/semantic"
	"github.com/memoria-space/meking/semantic/adapter/sqlite/internal/db"
)

func (s *Store) Open(
	ctx context.Context,
	namespace semantic.Namespace,
) (semantic.NamespaceReader, error) {
	if err := semantic.ValidateNamespace(namespace); err != nil {
		return nil, err
	}
	zoneID, err := semanticZoneID(ctx)
	if err != nil {
		return nil, err
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, classifySQLite("begin Semantic Namespace read", err)
	}
	closeTransaction := func() error {
		err := transaction.Rollback()
		if errors.Is(err, sql.ErrTxDone) {
			return nil
		}
		return classifySQLite("close Semantic Namespace read", err)
	}
	stm := newStatements(transaction)
	stored, err := stm.GetNamespace(ctx, db.GetNamespaceParams{
		ZoneID: zoneID, Name: string(namespace),
	})
	if err != nil {
		_ = closeTransaction()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", semantic.ErrNamespaceNotFound, namespace)
		}
		return nil, classifySQLite("read Semantic Namespace", err)
	}
	configured, err := vectorDatabase(ctx, stm)
	if err != nil {
		_ = closeTransaction()
		return nil, err
	}
	table := ""
	if configured.Dimension > 0 {
		table, err = existingVectorTable(ctx, transaction, int(configured.Dimension))
		if err != nil {
			_ = closeTransaction()
			return nil, err
		}
	}

	if configured.Dimension == 0 {
		_ = closeTransaction()
		return nil, errors.New("Semantic Namespace metadata is invalid")
	}

	invalid := int64(0)
	if table != "" {
		invalid, err = countInvalidNamespaceVectors(ctx, transaction, table, stored.ID)
		if err != nil {
			_ = closeTransaction()
			return nil, err
		}
	}
	if invalid != 0 {
		_ = closeTransaction()
		return nil, errors.New("Semantic Namespace is incomplete")
	}
	return &namespaceReader{
		transaction: transaction,
		statements:  stm,
		table:       table,
		rowID:       stored.ID,
		zoneID:      zoneID,
		info: semantic.Info{
			Namespace: namespace, Model: configured.Model,
			Dimension: int(configured.Dimension),
		},
	}, nil
}

type namespaceReader struct {
	transaction *sql.Tx
	statements  statements
	table       string
	rowID       int64
	zoneID      string
	info        semantic.Info

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func (r *namespaceReader) Info(ctx context.Context) (semantic.Info, error) {
	if r == nil {
		return semantic.Info{}, errors.New("read Semantic Namespace info: reader is not configured")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return semantic.Info{}, errors.New("read Semantic Namespace info: reader is closed")
	}
	if err := ctx.Err(); err != nil {
		return semantic.Info{}, err
	}
	return r.info, nil
}

func (r *namespaceReader) IDs(
	ctx context.Context,
	after string,
	limit int,
) (semantic.IDPage, error) {
	if r == nil {
		return semantic.IDPage{}, errors.New("read Semantic Namespace IDs: reader is not configured")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return semantic.IDPage{}, errors.New("read Semantic Namespace IDs: reader is closed")
	}
	if limit <= 0 {
		return semantic.IDPage{}, errors.New("Semantic Namespace ID page limit must be positive")
	}
	if after != "" {
		if err := semantic.ValidateID(after, "ID page boundary"); err != nil {
			return semantic.IDPage{}, err
		}
	}
	rows, err := r.statements.ListNamespaceVectorIDs(
		ctx,
		db.ListNamespaceVectorIDsParams{
			ZoneID: r.zoneID, NamespaceRowID: r.rowID,
			AfterID: after, PageLimit: int64(limit + 1),
		},
	)
	if err != nil {
		return semantic.IDPage{}, classifySQLite("read Semantic Namespace ID page", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	page := semantic.IDPage{Items: append([]string(nil), rows...), HasMore: hasMore}
	if len(page.Items) > 0 {
		page.NextAfter = page.Items[len(page.Items)-1]
	}
	return page, nil
}

func (r *namespaceReader) Search(
	ctx context.Context,
	vector []float64,
	limit int,
	filter map[string]string,
) ([]semantic.Match, error) {
	if r == nil {
		return nil, errors.New("search Semantic Namespace: reader is not configured")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, errors.New("search Semantic Namespace: reader is closed")
	}
	if limit <= 0 {
		return nil, errors.New("Semantic search limit must be positive")
	}
	if err := validateSemanticSearchFilter(r.zoneID, filter); err != nil {
		return nil, err
	}

	if len(vector) != r.info.Dimension {
		return nil, fmt.Errorf(
			"Semantic query dimension %d does not match Namespace dimension %d",
			len(vector), r.info.Dimension,
		)
	}
	normalized, err := semantic.NormalizeVector(vector)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}

	statement := fmt.Sprintf(`
		SELECT vector_id, distance
		FROM %q
		WHERE embedding MATCH vec_f32(?) AND k = ? AND namespace_row_id = ?
		ORDER BY distance
	`, r.table)
	rows, err := r.transaction.QueryContext(ctx, statement, string(encoded), limit, r.rowID)
	if err != nil {
		return nil, classifySQLite("search Semantic vec0 Namespace", err)
	}
	defer rows.Close()
	result := make([]semantic.Match, 0, limit)
	for rows.Next() {
		var id string
		var distance float64
		if err = rows.Scan(&id, &distance); err != nil {
			return nil, err
		}
		if err = semantic.ValidateID(id, "search result ID"); err != nil {
			return nil, err
		}
		if math.IsNaN(distance) || math.IsInf(distance, 0) {
			return nil, errors.New("Semantic vec0 returned a non-finite distance")
		}
		result = append(result, semantic.Match{ID: id, Score: max(-1, min(1, 1-distance))})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sortMatches(result)
	return result, nil
}

func sortMatches(matches []semantic.Match) {
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].Score != matches[right].Score {
			return matches[left].Score > matches[right].Score
		}
		return matches[left].ID < matches[right].ID
	})
}

func (r *namespaceReader) lookup(ctx context.Context, ids []string) ([]semantic.Vector, error) {
	result := make([]semantic.Vector, 0, len(ids))
	for _, id := range ids {
		row, err := r.statements.GetNamespaceVector(
			ctx, db.GetNamespaceVectorParams{
				ZoneID: r.zoneID, NamespaceRowID: r.rowID, VectorID: id,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, classifySQLite("lookup Semantic Namespace vector", err)
		}
		var values []float64
		if err := json.Unmarshal([]byte(row.Embedding), &values); err != nil {
			return nil, fmt.Errorf("decode Semantic vector %q: %w", row.VectorID, err)
		}
		if len(values) != r.info.Dimension {
			return nil, fmt.Errorf(
				"Semantic vector %q dimension %d does not match Namespace dimension %d",
				row.VectorID, len(values), r.info.Dimension,
			)
		}
		if row.ZoneID != r.zoneID {
			return nil, errors.New("Semantic Namespace vector Zone is inconsistent")
		}
		result = append(result, semantic.Vector{ID: row.VectorID, Values: values})
	}
	return result, nil
}

func (r *namespaceReader) Close() error {
	if r == nil || r.transaction == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closed = true
		err := r.transaction.Rollback()
		if errors.Is(err, sql.ErrTxDone) {
			err = nil
		}
		r.closeErr = classifySQLite("close Semantic Namespace reader", err)
	})
	return r.closeErr
}
