package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/memory/activation"
)

var _ activation.Store = (*Database)(nil)

func (database *Database) ReadState(ctx context.Context, subject knowledge.ObjectRef) (activation.StoredState, bool, error) {
	if err := activation.ValidateSubject(subject); err != nil {
		return activation.StoredState{}, false, err
	}
	executor, zoneID, err := database.scopedReader(ctx)
	if err != nil {
		return activation.StoredState{}, false, err
	}
	var stability, difficulty float64
	var lastReview, modelID string
	var sequence uint64
	err = executor.QueryRowContext(ctx, `SELECT stability,difficulty,last_review,applied_sequence,model_id FROM activation_states WHERE zone_id=? AND subject_kind=? AND subject_id=?`, zoneID, subjectKind(subject), subject.ObjectID()).Scan(&stability, &difficulty, &lastReview, &sequence, &modelID)
	if errors.Is(err, sql.ErrNoRows) {
		return activation.StoredState{}, false, nil
	}
	if err != nil {
		return activation.StoredState{}, false, classify(err)
	}
	at, err := time.Parse(time.RFC3339Nano, lastReview)
	if err != nil {
		return activation.StoredState{}, false, fmt.Errorf("%w: invalid last review", activation.ErrDataIntegrity)
	}
	state := activation.StoredState{Subject: subject, State: mas.State{Stability: mas.Stability(stability), Difficulty: mas.Difficulty(difficulty), LastReview: at}, Sequence: sequence, ModelID: modelID}
	return state, true, nil
}

func (database *Database) SaveState(ctx context.Context, state activation.StoredState) error {
	executor, zoneID, err := database.scopedWriter(ctx)
	if err != nil {
		return err
	}
	if err := activation.ValidateSubject(state.Subject); err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO activation_states (zone_id,subject_kind,subject_id,stability,difficulty,last_review,applied_sequence,model_id)
 VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(zone_id,subject_kind,subject_id) DO UPDATE SET
 stability=excluded.stability,difficulty=excluded.difficulty,last_review=excluded.last_review,applied_sequence=excluded.applied_sequence,model_id=excluded.model_id`,
		zoneID, subjectKind(state.Subject), state.Subject.ObjectID(), state.State.Stability.Float64(), state.State.Difficulty.Float64(), state.State.LastReview.UTC().Format(time.RFC3339Nano), state.Sequence, state.ModelID)
	return classify(err)
}

func (database *Database) InsertObservation(ctx context.Context, record activation.StoredObservation) error {
	executor, zoneID, err := database.scopedWriter(ctx)
	if err != nil {
		return err
	}
	input := record.Input
	if err := activation.ValidateSubject(input.Subject); err != nil {
		return err
	}
	encodedInput, err := encodeInput(input)
	if err != nil {
		return err
	}
	var sequence any
	if record.Receipt.AppliedSequence != 0 {
		sequence = record.Receipt.AppliedSequence
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO activation_observations (zone_id,observation_id,subject_kind,subject_id,version,occurred_at,protocol_digest,model_id,applied_sequence,input,recorded_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		zoneID, input.ID, subjectKind(input.Subject), input.Subject.ObjectID(), strconv.FormatUint(uint64(input.Version), 10), input.OccurredAt.Format(time.RFC3339Nano), input.ProtocolDigest, record.Receipt.ModelID, sequence, string(encodedInput), record.Receipt.RecordedAt.UTC().Format(time.RFC3339Nano))
	return classify(err)
}

const observationColumns = `observation_id,subject_kind,subject_id,version,occurred_at,protocol_digest,model_id,applied_sequence,input,recorded_at`

type scanner interface{ Scan(...any) error }

func scanObservation(row scanner) (activation.StoredObservation, error) {
	var id, kind, subjectID, version, occurredAt, protocolDigest, modelID, encodedInput, recordedAt string
	var sequence sql.NullInt64
	if err := row.Scan(&id, &kind, &subjectID, &version, &occurredAt, &protocolDigest, &modelID, &sequence, &encodedInput, &recordedAt); err != nil {
		return activation.StoredObservation{}, err
	}
	input, err := decodeInput(encodedInput)
	if err != nil {
		return activation.StoredObservation{}, err
	}
	at, err := time.Parse(time.RFC3339Nano, recordedAt)
	if err != nil {
		return activation.StoredObservation{}, fmt.Errorf("%w: invalid recording time", activation.ErrDataIntegrity)
	}
	receipt := activation.Receipt{ObservationID: id, Outcome: "recorded_unscorable", RecordedAt: at, ProtocolDigest: protocolDigest, ModelID: modelID}
	if sequence.Valid {
		if sequence.Int64 <= 0 {
			return activation.StoredObservation{}, activation.ErrDataIntegrity
		}
		receipt.Outcome = "applied"
		receipt.AppliedSequence = uint64(sequence.Int64)
	}
	record := activation.StoredObservation{Input: input, Receipt: receipt}
	if id != input.ID || kind != subjectKind(input.Subject) || subjectID != input.Subject.ObjectID() || version != strconv.FormatUint(uint64(input.Version), 10) || occurredAt != input.OccurredAt.Format(time.RFC3339Nano) || protocolDigest != input.ProtocolDigest || modelID != receipt.ModelID {
		return activation.StoredObservation{}, fmt.Errorf("%w: observation index differs from content", activation.ErrDataIntegrity)
	}
	return record, nil
}

func (database *Database) FindObservation(ctx context.Context, id string) (activation.StoredObservation, bool, error) {
	executor, zoneID, err := database.scopedReader(ctx)
	if err != nil {
		return activation.StoredObservation{}, false, err
	}
	record, err := scanObservation(executor.QueryRowContext(ctx, `SELECT `+observationColumns+` FROM activation_observations WHERE zone_id=? AND observation_id=?`, zoneID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return activation.StoredObservation{}, false, nil
	}
	if err != nil {
		return activation.StoredObservation{}, false, classify(err)
	}
	return record, true, nil
}

// AppliedObservations returns the immutable application order for audit and replay.
// It does not re-evaluate historical observations against today's Knowledge.
func (database *Database) AppliedObservations(ctx context.Context, subject knowledge.ObjectRef) ([]activation.StoredObservation, error) {
	if err := activation.ValidateSubject(subject); err != nil {
		return nil, err
	}
	executor, zoneID, err := database.scopedReader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT `+observationColumns+` FROM activation_observations WHERE zone_id=? AND subject_kind=? AND subject_id=? AND applied_sequence IS NOT NULL ORDER BY applied_sequence`, zoneID, subjectKind(subject), subject.ObjectID())
	if err != nil {
		return nil, classify(err)
	}
	defer rows.Close()
	result := []activation.StoredObservation{}
	for rows.Next() {
		record, err := scanObservation(rows)
		if err != nil {
			return nil, classify(err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, classify(err)
	}
	return result, nil
}
