package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/memory/activation"
	"github.com/memoria-space/meking/zone"
)

// ListObservations is the startup read projection across all stored Zones.
func (database *Database) ListObservations(ctx context.Context) ([]activation.ScopedObservation, error) {
	executor, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT zone_id,`+observationColumns+` FROM activation_observations ORDER BY zone_id,subject_kind,subject_id,applied_sequence`)
	if err != nil {
		return nil, classify(err)
	}
	defer rows.Close()
	result := []activation.ScopedObservation{}
	for rows.Next() {
		var id string
		record, err := scanObservation(prefixedScanner{row: rows, prefix: &id})
		if err != nil {
			return nil, classify(err)
		}
		result = append(result, activation.ScopedObservation{ZoneID: zone.ID(id), Observation: record})
	}
	return result, classify(rows.Err())
}

func (database *Database) ListStates(ctx context.Context) ([]activation.ScopedState, error) {
	executor, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT zone_id,subject_kind,subject_id,stability,difficulty,last_review,applied_sequence,model_id FROM activation_states ORDER BY zone_id,subject_kind,subject_id`)
	if err != nil {
		return nil, classify(err)
	}
	defer rows.Close()
	result := []activation.ScopedState{}
	for rows.Next() {
		var id, kind, subjectID, lastReview, modelID string
		var stability, difficulty float64
		var sequence uint64
		if err := rows.Scan(&id, &kind, &subjectID, &stability, &difficulty, &lastReview, &sequence, &modelID); err != nil {
			return nil, classify(err)
		}
		subject, err := restoreSubject(kind, subjectID)
		if err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339Nano, lastReview)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid last review encoding", activation.ErrDataIntegrity)
		}
		result = append(result, activation.ScopedState{ZoneID: zone.ID(id), State: activation.StoredState{Subject: subject, State: mas.State{Stability: mas.Stability(stability), Difficulty: mas.Difficulty(difficulty), LastReview: at}, Sequence: sequence, ModelID: modelID}})
	}
	return result, classify(rows.Err())
}

type prefixedScanner struct {
	row    scanner
	prefix *string
}

func (row prefixedScanner) Scan(values ...any) error {
	return row.row.Scan(append([]any{row.prefix}, values...)...)
}
