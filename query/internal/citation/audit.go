package citation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querysource "github.com/memoria-space/meking/query/source"
)

// Audit annotates model references using the final scope constructed by one
// Query aggregate. The aggregate remains responsible for deciding which rows
// survived selection, budgeting, Map/Reduce, or branch merging.
func Audit(
	ctx context.Context,
	response string,
	records []querybase.CitationRecord,
	sources querysource.Reader,
) (querybase.CitationAudit, error) {
	if sources == nil {
		return querybase.CitationAudit{}, errors.New("audit Query citations: Source Reader is required")
	}
	if err := ctx.Err(); err != nil {
		return querybase.CitationAudit{}, err
	}
	candidates, found := parse(response)
	audit := querybase.CitationAudit{
		Missing: !found,
		Items:   citationItems(candidates),
	}
	scope, err := indexRecords(records)
	if err != nil {
		return querybase.CitationAudit{}, err
	}
	if !found {
		return audit, nil
	}
	requests := make([]sourceRequest, 0)
	requestIndex := make(map[string]int)
	for index := range audit.Items {
		item := &audit.Items[index]
		if !item.Parsed {
			continue
		}
		record, exists := scope[item.Reference]
		if !exists {
			markInvalid(item, querybase.CitationOutsideContext)
			continue
		}
		position, exists := requestIndex[record.CorporaID]
		if !exists {
			position = len(requests)
			requestIndex[record.CorporaID] = position
			requests = append(requests, sourceRequest{
				corporaID: record.CorporaID,
				seen:      make(map[string]struct{}),
			})
		}
		for _, id := range record.TextUnitIDs {
			if _, duplicate := requests[position].seen[id]; duplicate {
				continue
			}
			requests[position].seen[id] = struct{}{}
			requests[position].textUnitIDs = append(requests[position].textUnitIDs, id)
		}
	}

	resolved := make(map[string][]querysource.TextUnitSource, len(requests))
	for _, request := range requests {
		values, err := sources.Read(
			ctx,
			request.corporaID,
			append([]string(nil), request.textUnitIDs...),
		)
		if err != nil {
			return querybase.CitationAudit{}, fmt.Errorf(
				"resolve Query citation sources for Corpora %q: %w",
				request.corporaID,
				err,
			)
		}
		for _, value := range values {
			if value.CorporaID != request.corporaID {
				return querybase.CitationAudit{}, fmt.Errorf(
					"resolve Query citation sources: reader returned Corpora %q for %q",
					value.CorporaID,
					request.corporaID,
				)
			}
		}
		resolved[request.corporaID] = append([]querysource.TextUnitSource(nil), values...)
	}
	for index := range audit.Items {
		item := &audit.Items[index]
		if !item.Parsed || item.Status == querybase.CitationInvalid {
			continue
		}
		record := scope[item.Reference]
		accepted := sourcesForRecord(resolved[record.CorporaID], record.TextUnitIDs)
		if len(accepted) == 0 {
			markInvalid(item, querybase.CitationUnresolvedSource)
			continue
		}
		item.Status = querybase.CitationValid
		item.Sources = accepted
	}
	return audit, nil
}

type sourceRequest struct {
	corporaID   string
	textUnitIDs []string
	seen        map[string]struct{}
}

func indexRecords(
	records []querybase.CitationRecord,
) (map[querybase.CitationReference]querybase.CitationRecord, error) {
	result := make(map[querybase.CitationReference]querybase.CitationRecord, len(records))
	for _, record := range records {
		if record.Reference.RecordID < 0 || !supportedDataset(record.Reference.Dataset) {
			return nil, errors.New("audit Query citations: record reference is invalid")
		}
		if _, duplicate := result[record.Reference]; duplicate {
			return nil, fmt.Errorf(
				"audit Query citations: record %s(%d) occurs more than once",
				record.Reference.Dataset,
				record.Reference.RecordID,
			)
		}
		if strings.TrimSpace(record.CorporaID) == "" {
			return nil, fmt.Errorf(
				"audit Query citations: record %s(%d) has no Corpora",
				record.Reference.Dataset,
				record.Reference.RecordID,
			)
		}
		ids, err := normalizeTextUnitIDs(record.TextUnitIDs)
		if err != nil {
			return nil, fmt.Errorf(
				"audit Query citations: record %s(%d): %w",
				record.Reference.Dataset,
				record.Reference.RecordID,
				err,
			)
		}
		record.TextUnitIDs = ids
		result[record.Reference] = record
	}
	return result, nil
}

func normalizeTextUnitIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, errors.New("TextUnit IDs are required")
	}
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return nil, errors.New("TextUnit ID is empty or has surrounding whitespace")
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func supportedDataset(dataset querybase.CitationDataset) bool {
	switch dataset {
	case querybase.CitationSources,
		querybase.CitationReports,
		querybase.CitationEntities,
		querybase.CitationRelationships,
		querybase.CitationClaims:
		return true
	default:
		return false
	}
}

func citationItems(candidates []candidate) []querybase.Citation {
	result := make([]querybase.Citation, len(candidates))
	for index, candidate := range candidates {
		result[index] = querybase.Citation{
			Raw: candidate.raw, Reference: candidate.reference, Parsed: candidate.parsed,
		}
		if !candidate.parsed {
			result[index].Status = querybase.CitationInvalid
			result[index].InvalidReason = candidate.invalidReason
		}
	}
	return result
}

func sourcesForRecord(
	sources []querysource.TextUnitSource,
	textUnitIDs []string,
) []querysource.TextUnitSource {
	acceptedIDs := make(map[string]struct{}, len(textUnitIDs))
	for _, id := range textUnitIDs {
		acceptedIDs[id] = struct{}{}
	}
	result := make([]querysource.TextUnitSource, 0)
	for _, current := range sources {
		if _, accepted := acceptedIDs[current.TextUnitID]; accepted {
			result = append(result, current)
		}
	}
	return result
}

func markInvalid(item *querybase.Citation, reason querybase.CitationInvalidReason) {
	item.Status = querybase.CitationInvalid
	item.InvalidReason = reason
	item.Sources = nil
}
