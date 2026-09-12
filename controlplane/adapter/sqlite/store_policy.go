package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
)

var _ controlapplication.PolicyStore = (*Store)(nil)

func (store *Store) LoadPolicy(
	ctx context.Context,
	action controlplane.Action,
) (controlplane.Policy, error) {
	if _, err := controlplane.ParseAction(string(action)); err != nil {
		return controlplane.Policy{}, err
	}
	var mode string
	var minimumPending int64
	var maximumWaitNanoseconds int64
	var revision int64
	var updatedAt string
	reader, err := store.reader(ctx)
	if err != nil {
		return controlplane.Policy{}, err
	}
	err = reader.QueryRowContext(
		ctx,
		`SELECT mode, minimum_pending, maximum_wait_nanoseconds, revision, updated_at
		 FROM controlplane_policies WHERE action = ?`,
		string(action),
	).Scan(
		&mode,
		&minimumPending,
		&maximumWaitNanoseconds,
		&revision,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return controlplane.Policy{}, controlplane.ErrPolicyNotFound
	}
	if err != nil {
		return controlplane.Policy{}, fmt.Errorf("read %s Policy: %w", action, err)
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return controlplane.Policy{}, fmt.Errorf("restore %s Policy UpdatedAt: %w", action, err)
	}
	return controlplane.RestorePolicy(controlplane.Policy{
		Action:         action,
		Mode:           controlplane.PolicyMode(mode),
		MinimumPending: uint64(minimumPending),
		MaximumWait:    time.Duration(maximumWaitNanoseconds),
		Revision:       uint64(revision),
		UpdatedAt:      publishedAt,
	})
}

func (store *Store) SavePolicy(
	ctx context.Context,
	expectedRevision uint64,
	policy controlplane.Policy,
) error {
	validated, err := controlplane.RestorePolicy(policy)
	if err != nil {
		return err
	}
	if validated.Revision > math.MaxInt64 ||
		validated.MinimumPending > math.MaxInt64 ||
		validated.MaximumWait > time.Duration(math.MaxInt64) {
		return controlplane.ErrInvalidPolicy
	}
	executor, err := store.writer(ctx)
	if err != nil {
		return err
	}
	if expectedRevision == 0 {
		result, err := executor.ExecContext(
			ctx,
			`INSERT INTO controlplane_policies (
			    action, mode, minimum_pending, maximum_wait_nanoseconds, revision, updated_at
			 ) VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(action) DO NOTHING`,
			string(validated.Action),
			string(validated.Mode),
			int64(validated.MinimumPending),
			int64(validated.MaximumWait),
			int64(validated.Revision),
			validated.UpdatedAt.Format(time.RFC3339Nano),
		)
		return classifyPolicyWrite(result, err, validated.Action)
	}
	if expectedRevision > math.MaxInt64 || validated.Revision != expectedRevision+1 {
		return controlplane.ErrPolicyConflict
	}
	result, err := executor.ExecContext(
		ctx,
		`UPDATE controlplane_policies
		 SET mode = ?, minimum_pending = ?, maximum_wait_nanoseconds = ?, revision = ?, updated_at = ?
		 WHERE action = ? AND revision = ?`,
		string(validated.Mode),
		int64(validated.MinimumPending),
		int64(validated.MaximumWait),
		int64(validated.Revision),
		validated.UpdatedAt.Format(time.RFC3339Nano),
		string(validated.Action),
		int64(expectedRevision),
	)
	return classifyPolicyWrite(result, err, validated.Action)
}

func (store *Store) ListPolicies(ctx context.Context) ([]controlplane.Policy, error) {
	reader, err := store.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := reader.QueryContext(
		ctx,
		`SELECT action, mode, minimum_pending, maximum_wait_nanoseconds, revision, updated_at
		 FROM controlplane_policies ORDER BY action`,
	)
	if err != nil {
		return nil, fmt.Errorf("list Control Policies: %w", err)
	}
	defer rows.Close()
	result := make([]controlplane.Policy, 0, len(controlplane.Actions()))
	for rows.Next() {
		var action string
		var mode string
		var minimumPending int64
		var maximumWaitNanoseconds int64
		var revision int64
		var updatedAt string
		if err := rows.Scan(
			&action,
			&mode,
			&minimumPending,
			&maximumWaitNanoseconds,
			&revision,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan Control Policy: %w", err)
		}
		parsedAction, err := controlplane.ParseAction(action)
		if err != nil {
			return nil, err
		}
		publishedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("restore %s Policy UpdatedAt: %w", action, err)
		}
		policy, err := controlplane.RestorePolicy(controlplane.Policy{
			Action:         parsedAction,
			Mode:           controlplane.PolicyMode(mode),
			MinimumPending: uint64(minimumPending),
			MaximumWait:    time.Duration(maximumWaitNanoseconds),
			Revision:       uint64(revision),
			UpdatedAt:      publishedAt,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Control Policies: %w", err)
	}
	return result, nil
}

func classifyPolicyWrite(
	result sql.Result,
	err error,
	action controlplane.Action,
) error {
	if err != nil {
		return fmt.Errorf("save %s Policy: %w", action, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read %s Policy write count: %w", action, err)
	}
	if updated != 1 {
		return controlplane.ErrPolicyConflict
	}
	return nil
}
