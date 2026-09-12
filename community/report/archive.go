package report

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/memoria-space/meking/community"
)

// Store is the Report subdomain's persistence port for immutable Reports.
// Archive calls Insert only after validating the non-empty batch and unique
// IDs. One Insert atomically creates that complete batch and rejects every
// existing ID. LoadReports reads one non-empty, unique ID batch in caller order
// through one storage read boundary. Implementations never generate Reports,
// compare reuse inputs, or choose query visibility.
type Store interface {
	Insert(ctx context.Context, reports []Report) error
	LoadReports(ctx context.Context, ids []ID) ([]Report, error)
}

// Archive validates, atomically stores, and reads immutable Reports
// without depending on SQLite, Project paths, ReportSet, or process runtime.
type Archive struct {
	// store is the fact source for immutable Report rows and ordered sources.
	store Store
	// logger records identities and counts but never prompt or Report content.
	logger community.CommunityLogger
}

// NewArchive creates the Report application API from persistence and
// structured-logging dependencies.
func NewArchive(
	store Store,
	logger community.CommunityLogger,
) (*Archive, error) {
	if store == nil {
		return nil, errors.New("create Report Archive: store is required")
	}
	if logger == nil {
		return nil, errors.New("create Report Archive: logger is required")
	}
	return &Archive{store: store, logger: logger}, nil
}

// Save atomically persists one complete generation batch. Every Report must be
// valid and IDs must be unique before the Store transaction starts.
func (a *Archive) Save(ctx context.Context, reports []Report) error {
	if len(reports) == 0 {
		err := fmt.Errorf("%w: Report batch must not be empty", ErrInvalidReport)
		a.logFailure(ctx, "Report batch rejected", err)
		return err
	}
	seen := make(map[ID]struct{}, len(reports))
	for _, report := range reports {
		if err := ValidateReport(report); err != nil {
			a.logFailure(ctx, "Report batch rejected", err)
			return err
		}
		if _, duplicate := seen[report.ID]; duplicate {
			err := fmt.Errorf(
				"%w: duplicate Report ID %q in one batch",
				ErrInvalidReport,
				report.ID,
			)
			a.logFailure(ctx, "Report batch rejected", err)
			return err
		}
		seen[report.ID] = struct{}{}
	}
	if err := a.store.Insert(ctx, reports); err != nil {
		err = fmt.Errorf("save Report batch: %w", err)
		a.logFailure(ctx, "Report batch save failed", err)
		return err
	}
	a.logger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Report batch saved",
		slog.Int("reports", len(reports)),
	)
	return nil
}

// Report returns one exact immutable Report without consulting ReportSet
// visibility or current Knowledge.
func (a *Archive) Report(
	ctx context.Context,
	id ID,
) (Report, error) {
	reports, err := a.Reports(ctx, []ID{id})
	if err != nil {
		return Report{}, err
	}
	return reports[0], nil
}

// Reports returns one ordered batch of exact immutable Reports without
// consulting ReportSet visibility. IDs must be non-empty and unique; a missing,
// reordered, or invalid stored value rejects the complete batch.
func (a *Archive) Reports(
	ctx context.Context,
	ids []ID,
) ([]Report, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf(
			"%w: Report read batch must not be empty",
			ErrInvalidReport,
		)
	}
	seen := make(map[ID]struct{}, len(ids))
	for _, id := range ids {
		if err := ValidateID(id); err != nil {
			return nil, err
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf(
				"%w: duplicate Report ID %q in read batch",
				ErrInvalidReport,
				id,
			)
		}
		seen[id] = struct{}{}
	}
	reports, err := a.store.LoadReports(ctx, append([]ID(nil), ids...))
	if err != nil {
		return nil, err
	}
	if len(reports) != len(ids) {
		return nil, fmt.Errorf(
			"%w: Report Store returned %d values for %d IDs",
			ErrReportDataIntegrity,
			len(reports),
			len(ids),
		)
	}
	for index, report := range reports {
		if report.ID != ids[index] {
			return nil, fmt.Errorf(
				"%w: Report Store returned ID %q at position %d; expected %q",
				ErrReportDataIntegrity,
				report.ID,
				index,
				ids[index],
			)
		}
		if err := ValidateReport(report); err != nil {
			return nil, fmt.Errorf(
				"%w: Report %q is invalid: %v",
				ErrReportDataIntegrity,
				report.ID,
				err,
			)
		}
	}
	return reports, nil
}

func (a *Archive) logFailure(
	ctx context.Context,
	message string,
	err error,
) {
	level := slog.LevelError
	if errors.Is(err, ErrInvalidReport) ||
		errors.Is(err, ErrReportConflict) ||
		errors.Is(err, ErrReportNotFound) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		level = slog.LevelWarn
	}
	a.logger.LogAttrs(ctx, level, message, slog.Any("error", err))
}
