package sqlite

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
)

func (store *Store) RecordTextUnitVectors(ctx context.Context, completion corpus.TextVectorCompletion) (bool, error) {
	executor, err := transactionsqlite.Current(ctx, store.database)
	if err != nil {
		return false, fmt.Errorf("join Corpus vector completion transaction: %w", err)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return false, err
	}
	corporaID, err := corpus.CorporaIDSequence(completion.CorporaID)
	if err != nil {
		return false, err
	}
	if _, err := executor.ExecContext(ctx, `
		INSERT OR IGNORE INTO corpus_text_vector_progress (
			zone_id, corpora_id, text_id, source_event_id
		) VALUES (?, ?, ?, ?)
	`, string(zoneID), corporaID, string(completion.TextID), string(completion.EventID)); err != nil {
		return false, err
	}
	var storedEvent string
	if err := executor.QueryRowContext(ctx, `
		SELECT source_event_id FROM corpus_text_vector_progress
		WHERE zone_id = ? AND corpora_id = ? AND text_id = ?
	`, string(zoneID), corporaID, string(completion.TextID)).Scan(&storedEvent); err != nil || storedEvent != string(completion.EventID) {
		return false, corpus.ErrCorporaPreparationConflict
	}
	var incomplete int64
	if err := executor.QueryRowContext(ctx, `
		SELECT count(*) FROM text_chunking_progress
		WHERE zone_id = ? AND completed = 0
	`, string(zoneID)).Scan(&incomplete); err != nil {
		return false, err
	}
	if incomplete != 0 {
		return false, nil
	}
	var spanCount int64
	if err := executor.QueryRowContext(ctx, `
		SELECT count(*) FROM text_unit_spans WHERE zone_id = ? AND corpora_id = ?
	`, string(zoneID), corporaID).Scan(&spanCount); err != nil {
		return false, err
	}
	return spanCount > 0, nil
}
