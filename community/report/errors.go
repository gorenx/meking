package report

import "errors"

var (
	// ErrInvalidReport identifies Report evidence or an immutable Report that
	// violates identity, source-closure, ordering, or structured-result rules.
	ErrInvalidReport = errors.New("community report: invalid Report")
	// ErrReportConflict identifies an immutable Report ID that is already stored.
	// Callers must not overwrite it even when a retry supplies equal content.
	ErrReportConflict = errors.New("community report: Report identity conflict")
	// ErrReportNotFound identifies an immutable Report ID absent from the
	// Community Store.
	ErrReportNotFound = errors.New("community report: Report not found")
	// ErrReportDataIntegrity identifies persisted Report rows that cannot
	// reconstruct one valid immutable Report.
	ErrReportDataIntegrity = errors.New("community report: Report data integrity failure")
	// ErrReportInputTooLarge identifies evidence or fragment output that cannot
	// fit a required model call within MaxInputTokens. No partial Report is returned.
	ErrReportInputTooLarge = errors.New("community report: Report input cannot be completed within token budget")
	// ErrInvalidReportSet identifies an incomplete Report selection or Corpus
	// evidence mismatch.
	ErrInvalidReportSet = errors.New("community report: invalid ReportSet")
	// ErrReportSetConflict identifies an immutable ReportSet ID that is already
	// stored and therefore cannot be overwritten.
	ErrReportSetConflict = errors.New("community report: ReportSet identity conflict")
	// ErrReportSetNotFound identifies a requested ReportSet or current selection
	// that does not exist.
	ErrReportSetNotFound = errors.New("community report: ReportSet not found")
)
