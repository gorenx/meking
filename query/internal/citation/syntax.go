// Package citation provides the shared, internal grammar and source-audit
// algorithm invoked by Basic, Local, Global, and DRIFT aggregate services.
package citation

import (
	"fmt"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
)

const MaximumRecordIDs = 5

// ParseReferences returns every syntactically valid request-local reference in
// model order. Found distinguishes text without a Citation marker from a
// malformed marker; malformed syntax returns a stable error without copying
// model output into the error. Query aggregates use this before accepting
// intermediate model results whose evidence scope is narrower than the final
// Citation audit scope.
func ParseReferences(
	text string,
) (references []querybase.CitationReference, found bool, err error) {
	candidates, found := parse(text)
	references = make([]querybase.CitationReference, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.parsed {
			return nil, found, fmt.Errorf(
				"citation syntax is invalid: %s",
				candidate.invalidReason,
			)
		}
		references = append(references, candidate.reference)
	}
	return references, found, nil
}

type candidate struct {
	raw           string
	reference     querybase.CitationReference
	parsed        bool
	invalidReason querybase.CitationInvalidReason
}

func parse(response string) ([]candidate, bool) {
	const marker = "[Data:"
	result := make([]candidate, 0)
	found := false
	for cursor := 0; cursor < len(response); {
		relativeStart := strings.Index(response[cursor:], marker)
		if relativeStart < 0 {
			break
		}
		found = true
		start := cursor + relativeStart
		contentStart := start + len(marker)
		relativeEnd := strings.IndexByte(response[contentStart:], ']')
		if relativeEnd < 0 {
			result = append(result, invalid(response[start:], querybase.CitationMalformed))
			break
		}
		end := contentStart + relativeEnd
		raw := response[start : end+1]
		result = append(result, parseBlock(raw, response[contentStart:end])...)
		cursor = end + 1
	}
	return result, found
}

func parseBlock(raw string, content string) []candidate {
	groups, valid := splitGroups(content)
	if !valid || len(groups) == 0 {
		return []candidate{invalid(raw, querybase.CitationMalformed)}
	}
	result := make([]candidate, 0)
	for _, group := range groups {
		result = append(result, parseGroup(group)...)
	}
	return result
}

func splitGroups(content string) ([]string, bool) {
	groups := make([]string, 0)
	start := 0
	depth := 0
	for index := 0; index < len(content); index++ {
		switch content[index] {
		case '(':
			depth++
			if depth > 1 {
				return nil, false
			}
		case ')':
			depth--
			if depth < 0 {
				return nil, false
			}
		case ',', ';':
			if depth == 0 {
				group := strings.TrimSpace(content[start:index])
				if group == "" {
					return nil, false
				}
				groups = append(groups, group)
				start = index + 1
			}
		}
	}
	if depth != 0 {
		return nil, false
	}
	group := strings.TrimSpace(content[start:])
	if group == "" {
		return nil, false
	}
	return append(groups, group), true
}

func parseGroup(group string) []candidate {
	open := strings.IndexByte(group, '(')
	close := strings.LastIndexByte(group, ')')
	if open <= 0 || close <= open || strings.TrimSpace(group[close+1:]) != "" {
		return []candidate{invalid(group, querybase.CitationMalformed)}
	}
	dataset, valid := parseDataset(group[:open])
	if !valid {
		return []candidate{invalid(group, querybase.CitationUnsupportedDataset)}
	}
	tokens := strings.Split(group[open+1:close], ",")
	recordIDs := make([]int, 0, len(tokens))
	for index, token := range tokens {
		token = strings.TrimSpace(token)
		if strings.EqualFold(token, "+more") {
			if index != len(tokens)-1 || len(recordIDs) == 0 {
				return []candidate{invalid(group, querybase.CitationInvalidRecordID)}
			}
			continue
		}
		if !decimalDigits(token) {
			return []candidate{invalid(group, querybase.CitationInvalidRecordID)}
		}
		recordID, err := strconv.Atoi(token)
		if err != nil {
			return []candidate{invalid(group, querybase.CitationInvalidRecordID)}
		}
		recordIDs = append(recordIDs, recordID)
	}
	if len(recordIDs) == 0 {
		return []candidate{invalid(group, querybase.CitationInvalidRecordID)}
	}
	if len(recordIDs) > MaximumRecordIDs {
		return []candidate{invalid(group, querybase.CitationTooManyRecords)}
	}
	result := make([]candidate, 0, len(recordIDs))
	for _, recordID := range recordIDs {
		result = append(result, candidate{
			raw: group,
			reference: querybase.CitationReference{
				Dataset: dataset, RecordID: recordID,
			},
			parsed: true,
		})
	}
	return result
}

func parseDataset(value string) (querybase.CitationDataset, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sources":
		return querybase.CitationSources, true
	case "reports":
		return querybase.CitationReports, true
	case "entities":
		return querybase.CitationEntities, true
	case "relationships":
		return querybase.CitationRelationships, true
	case "claims":
		return querybase.CitationClaims, true
	default:
		return "", false
	}
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func invalid(raw string, reason querybase.CitationInvalidReason) candidate {
	return candidate{raw: raw, invalidReason: reason}
}

// RewriteReferences replaces request-local references with IDs assigned by an
// owning aggregate. Citation blocks without any accepted mapping are removed,
// so stale branch-local IDs cannot enter a later model stage.
func RewriteReferences(
	text string,
	replacements map[querybase.CitationReference]querybase.CitationReference,
) string {
	const marker = "[Data:"
	var result strings.Builder
	for cursor := 0; cursor < len(text); {
		relativeStart := strings.Index(text[cursor:], marker)
		if relativeStart < 0 {
			result.WriteString(text[cursor:])
			break
		}
		start := cursor + relativeStart
		result.WriteString(text[cursor:start])
		contentStart := start + len(marker)
		relativeEnd := strings.IndexByte(text[contentStart:], ']')
		if relativeEnd < 0 {
			break
		}
		end := contentStart + relativeEnd
		mapped := mappedReferences(
			parseBlock(text[start:end+1], text[contentStart:end]),
			replacements,
		)
		if len(mapped) > 0 {
			result.WriteString(renderReferences(mapped))
		}
		cursor = end + 1
	}
	return result.String()
}

func mappedReferences(
	candidates []candidate,
	replacements map[querybase.CitationReference]querybase.CitationReference,
) []querybase.CitationReference {
	result := make([]querybase.CitationReference, 0, len(candidates))
	seen := make(map[querybase.CitationReference]struct{}, len(candidates))
	for _, current := range candidates {
		if !current.parsed {
			continue
		}
		mapped, exists := replacements[current.reference]
		if !exists {
			continue
		}
		if _, duplicate := seen[mapped]; duplicate {
			continue
		}
		seen[mapped] = struct{}{}
		result = append(result, mapped)
	}
	return result
}

func renderReferences(references []querybase.CitationReference) string {
	datasets := make([]querybase.CitationDataset, 0)
	ids := make(map[querybase.CitationDataset][]int)
	for _, reference := range references {
		if _, exists := ids[reference.Dataset]; !exists {
			datasets = append(datasets, reference.Dataset)
		}
		ids[reference.Dataset] = append(ids[reference.Dataset], reference.RecordID)
	}
	groups := make([]string, 0)
	for _, dataset := range datasets {
		values := ids[dataset]
		for start := 0; start < len(values); start += MaximumRecordIDs {
			end := min(start+MaximumRecordIDs, len(values))
			recordIDs := make([]string, end-start)
			for index, recordID := range values[start:end] {
				recordIDs[index] = strconv.Itoa(recordID)
			}
			groups = append(groups, fmt.Sprintf(
				"%s (%s)", dataset, strings.Join(recordIDs, ", "),
			))
		}
	}
	return "[Data: " + strings.Join(groups, "; ") + "]"
}
