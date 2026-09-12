package corpus

import (
	"fmt"
	"strings"

	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
)

func ValidateTextUnitContent(value text.Text, unit textunits.TextUnit) error {
	if err := textunits.ValidateTextUnitContent(value, unit); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorpus, err)
	}
	return nil
}

func validateTextUnit(unit textunits.TextUnit) error {
	return textunits.ValidateTextUnit(unit)
}

func ValidateTextUnitLocation(location TextUnitLocation) error {
	if err := ValidateCorporaID(location.CorporaID); err != nil {
		return err
	}
	if err := text.ValidateID(location.TextID); err != nil {
		return err
	}
	if strings.TrimSpace(location.TextTitle) == "" {
		return fmt.Errorf("%w: Text title is required", ErrInvalidCorpus)
	}
	if err := document.ValidateID(location.DocumentID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCorpus, err)
	}
	if location.DocumentLocation == "" {
		return fmt.Errorf("%w: Document Location is required", ErrInvalidCorpus)
	}
	if err := validateTextUnit(location.TextUnit); err != nil {
		return fmt.Errorf("%w: %v", ErrCorpusDataIntegrity, err)
	}
	return nil
}
