package report

import (
	"fmt"

	"github.com/memoria-space/meking/internal/uuid"
)

func newReportID() (ID, error) {
	value, err := uuid.NewV4()
	return ID(value), err
}

func newReportSetID() (ReportSetID, error) {
	value, err := uuid.NewV4()
	return ReportSetID(value), err
}

// ValidateID verifies the canonical lowercase UUID v4 used to identify
// one immutable Report. Report content and reuse eligibility are validated
// independently from identity.
func ValidateID(id ID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf(
			"%w: Report ID %q is not a canonical lowercase UUID v4",
			ErrInvalidReport,
			id,
		)
	}
	return nil
}

// ValidateReportSetID verifies the canonical lowercase UUID v4 allocated for
// one immutable ReportSet. The ID has no ordering or content-hash meaning.
func ValidateReportSetID(id ReportSetID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf(
			"%w: ReportSet ID %q is not a canonical lowercase UUID v4",
			ErrInvalidReportSet,
			id,
		)
	}
	return nil
}
