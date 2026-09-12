package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/memoria-space/meking/community"
)

// ReportSetID identifies one immutable, complete Community-to-Report
// selection. It is a canonical lowercase UUID v4 and has no ordering,
// Knowledge Version, or content-hash meaning.
type ReportSetID string

// ReportSet is one complete publication candidate that binds a CommunitySet
// to exactly one immutable Report per Community and fixes the Corpora used to
// resolve every TextUnit citation. Publication is selected separately.
type ReportSet struct {
	// ID is allocated after the supplied Reports and Corpus evidence pass
	// validation. It never identifies another ReportSet value.
	ID ReportSetID
	// CommunitySetID identifies the complete immutable hierarchy covered by
	// Reports.
	CommunitySetID community.CommunitySetID
	// CorporaID is the immutable Corpus identity fixed before publication.
	// Community stores it as an opaque cross-domain identity.
	CorporaID string
	// Reports maps every CommunityID in CommunitySetID to exactly one immutable
	// Report ID. Map iteration order has no meaning.
	Reports map[community.CommunityID]ID
	// CreatedAt is the UTC time when the complete candidate passed validation.
	// It is diagnostic and does not participate in identity or publication order.
	CreatedAt time.Time
}

// NewReportSet binds a generated Report collection to the CommunitySet and
// Corpora that produced it. The persistence boundary verifies exact Community
// coverage against the stored CommunitySet before accepting the candidate.
// availableTextUnits is a set keyed by canonical TextUnit ID projected from the
// exact Corpora identified by corporaID.
func NewReportSet(
	communitySetID community.CommunitySetID,
	corporaID string,
	reports []Report,
	availableTextUnits map[string]struct{},
) (ReportSet, error) {
	if err := community.ValidateCommunitySetID(communitySetID); err != nil {
		return ReportSet{}, fmt.Errorf("%w: %v", ErrInvalidReportSet, err)
	}
	if strings.TrimSpace(corporaID) == "" {
		return ReportSet{}, fmt.Errorf("%w: Corpora ID is required", ErrInvalidReportSet)
	}
	if len(reports) == 0 {
		return ReportSet{}, fmt.Errorf("%w: Reports must not be empty", ErrInvalidReportSet)
	}

	selected := make(map[community.CommunityID]ID, len(reports))
	for _, current := range reports {
		if err := ValidateReport(current); err != nil {
			return ReportSet{}, fmt.Errorf("%w: %v", ErrInvalidReportSet, err)
		}
		if _, duplicate := selected[current.CommunityID]; duplicate {
			return ReportSet{}, fmt.Errorf(
				"%w: Community %q has more than one Report",
				ErrInvalidReportSet,
				current.CommunityID,
			)
		}
		for _, textUnitID := range current.TextUnitIDs {
			if _, found := availableTextUnits[textUnitID]; !found {
				return ReportSet{}, fmt.Errorf(
					"%w: Report %q TextUnit %q is absent from Corpora %q",
					ErrInvalidReportSet,
					current.ID,
					textUnitID,
					corporaID,
				)
			}
		}
		selected[current.CommunityID] = current.ID
	}
	id, err := newReportSetID()
	if err != nil {
		return ReportSet{}, fmt.Errorf("create ReportSet ID: %w", err)
	}
	set := ReportSet{
		ID:             id,
		CommunitySetID: communitySetID,
		CorporaID:      corporaID,
		Reports:        selected,
		CreatedAt:      time.Now().UTC(),
	}
	if err := ValidateReportSet(set); err != nil {
		return ReportSet{}, err
	}
	return set, nil
}

// ValidateReportSet checks the self-contained identity and mapping invariants
// available without loading the referenced CommunitySet, Reports, or Corpora.
func ValidateReportSet(set ReportSet) error {
	if err := ValidateReportSetID(set.ID); err != nil {
		return err
	}
	if err := community.ValidateCommunitySetID(set.CommunitySetID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidReportSet, err)
	}
	if strings.TrimSpace(set.CorporaID) == "" {
		return fmt.Errorf("%w: Corpora ID is required", ErrInvalidReportSet)
	}
	if len(set.Reports) == 0 {
		return fmt.Errorf("%w: Reports must not be empty", ErrInvalidReportSet)
	}
	reportIDs := make(map[ID]struct{}, len(set.Reports))
	for communityID, reportID := range set.Reports {
		if err := community.ValidateCommunityID(communityID); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidReportSet, err)
		}
		if err := ValidateID(reportID); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidReportSet, err)
		}
		if _, duplicate := reportIDs[reportID]; duplicate {
			return fmt.Errorf(
				"%w: Report %q is selected more than once",
				ErrInvalidReportSet,
				reportID,
			)
		}
		reportIDs[reportID] = struct{}{}
	}
	if set.CreatedAt.IsZero() || set.CreatedAt.Location() != time.UTC {
		return fmt.Errorf(
			"%w: CreatedAt must be a non-zero UTC time",
			ErrInvalidReportSet,
		)
	}
	return nil
}
