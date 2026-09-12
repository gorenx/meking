package corpus

import (
	"fmt"
	"strconv"
)

func ValidateCorporaID(id CorporaID) error {
	_, err := CorporaIDSequence(id)
	return err
}

func FormatCorporaID(id CorporaID) string {
	return string(id)
}

func ParseCorporaID(value string) (CorporaID, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return "", fmt.Errorf("%w: Corpora ID is invalid", ErrInvalidCorpus)
	}
	if parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
		return "", fmt.Errorf("%w: Corpora ID is not a positive canonical integer", ErrInvalidCorpus)
	}
	return CorporaID(value), nil
}

func NewCorporaID(sequence int64) (CorporaID, error) {
	if sequence <= 0 {
		return "", fmt.Errorf("%w: Corpora ID must be positive", ErrInvalidCorpus)
	}
	return CorporaID(strconv.FormatInt(sequence, 10)), nil
}

func CorporaIDSequence(id CorporaID) (int64, error) {
	parsed, err := strconv.ParseInt(string(id), 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != string(id) {
		return 0, fmt.Errorf("%w: Corpora ID is not a positive canonical integer", ErrInvalidCorpus)
	}
	return parsed, nil
}
