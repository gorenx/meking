package merger

import (
	"strconv"
	"strings"
)

type Input struct {
	SourceCorporaID string
}

func parseCorporaID(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || strings.TrimSpace(value) != value || strconv.FormatInt(parsed, 10) != value {
		return 0, ErrInvalidInput
	}
	return parsed, nil
}
