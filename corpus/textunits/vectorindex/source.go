// Package vectorindex owns the unique TextUnit texts indexed for one Corpora.
// TextUnit occurrences remain Corpora TextUnitSpan facts and are not copied
// into Semantic.
package vectorindex

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/memoria-space/meking/corpus/textunits"
)

const PageSize = 256

var ErrNotFound = errors.New("TextUnit vector Namespace not found")

type Source struct {
	ID   string
	Text string
}

func Namespace(corporaID string) (string, error) {
	if strings.TrimSpace(corporaID) == "" || strings.TrimSpace(corporaID) != corporaID {
		return "", errors.New("TextUnit vector CorporaID is invalid")
	}
	return corporaID, nil
}

// Sources collapses repeated occurrences of the same TextUnit ID. Repeated IDs
// must retain exactly the same complete TextUnit text.
func Sources(units []textunits.TextUnitBody) ([]Source, error) {
	byID := make(map[string]string, len(units))
	for index, unit := range units {
		id := string(unit.ID)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return nil, fmt.Errorf("TextUnit vector source %d has an invalid ID", index)
		}
		if strings.TrimSpace(unit.Text) == "" {
			return nil, fmt.Errorf("TextUnit vector source %q has empty text", id)
		}
		if existing, duplicate := byID[id]; duplicate && existing != unit.Text {
			return nil, fmt.Errorf("TextUnit %q occurrences contain different text", id)
		}
		byID[id] = unit.Text
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]Source, len(ids))
	for index, id := range ids {
		result[index] = Source{ID: id, Text: byID[id]}
	}
	return result, nil
}
