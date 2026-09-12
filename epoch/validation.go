package epoch

import (
	"fmt"
	"strings"
	"time"

	"github.com/memoria-space/meking/internal/uuid"
)

// ValidateEpoch checks the self-contained value rules available without
// loading Corpus, Community, Knowledge, or vector facts.
func ValidateEpoch(value Epoch) error {
	if value.ID <= 0 {
		return fmt.Errorf("%w: ID must be positive", ErrInvalidEpoch)
	}
	if err := value.Knowledge.Validate(); err != nil {
		return fmt.Errorf("%w: invalid Knowledge Versions: %v", ErrInvalidEpoch, err)
	}
	if err := validateCorporaID(value.CorporaID); err != nil {
		return err
	}
	if err := validateStructureID(value.StructureID); err != nil {
		return err
	}
	if value.PublishedAt.IsZero() || value.PublishedAt.Location() != time.UTC {
		return fmt.Errorf("%w: PublishedAt must be a non-zero UTC time", ErrInvalidEpoch)
	}
	return nil
}

// ValidatePublicationTarget checks the baseline sentinel and Knowledge upper
// bound fixed by an Epoch Publication.
func ValidatePublicationTarget(value PublicationTarget) error {
	if value.ExpectedEpoch < 0 {
		return fmt.Errorf("%w: ExpectedEpoch must be non-negative", ErrInvalidEpoch)
	}
	if err := value.Knowledge.Validate(); err != nil {
		return fmt.Errorf("%w: invalid Knowledge Versions: %v", ErrInvalidEpoch, err)
	}
	if err := validateStructureID(value.StructureID); err != nil {
		return err
	}
	return nil
}

// ValidatePublication checks all caller-supplied values before readiness or a
// persistence transaction. The Store still validates the allocated Epoch after
// assigning its positive ID.
func ValidatePublication(
	target PublicationTarget,
	corporaID CorporaID,
	publishedAt time.Time,
) error {
	if err := ValidatePublicationTarget(target); err != nil {
		return err
	}
	if err := validateCorporaID(corporaID); err != nil {
		return err
	}
	if publishedAt.IsZero() || publishedAt.Location() != time.UTC {
		return fmt.Errorf("%w: PublishedAt must be a non-zero UTC time", ErrInvalidEpoch)
	}
	return nil
}

func validateCorporaID(id CorporaID) error {
	value := string(id)
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%w: Corpora ID is invalid", ErrInvalidEpoch)
	}
	return nil
}

func validateStructureID(id StructureID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf(
			"%w: Structure ID %q is not a canonical lowercase UUID v4",
			ErrInvalidEpoch,
			id,
		)
	}
	return nil
}
