package report

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/memoria-space/meking/community"
)

var (
	ErrPublicationNotFound = errors.New("Community Report publication not found")
	ErrPublicationConflict = errors.New("Community Report publication conflicts with committed state")
)

type Publication struct {
	EpochID      int64
	StructureID  community.StructureID
	ReportSetID  ReportSetID
	VectorsReady bool
	CreatedAt    time.Time
}

func (publication Publication) Validate() error {
	if publication.EpochID <= 0 {
		return fmt.Errorf("invalid Community Report publication: EpochID must be positive")
	}
	if _, err := community.RestoreStructureID(publication.StructureID); err != nil {
		return err
	}
	if err := ValidateReportSetID(publication.ReportSetID); err != nil {
		return err
	}
	if publication.CreatedAt.IsZero() || publication.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("invalid Community Report publication: CreatedAt must be UTC")
	}
	return nil
}

type PublicationStore interface {
	LoadReportPublication(context.Context, community.StructureID) (Publication, error)
	PrepareReportPublication(context.Context, Publication) error
	CompleteReportPublication(context.Context, community.StructureID, ReportSetID) error
}
