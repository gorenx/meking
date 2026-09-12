// Package qctx renders aggregate-selected evidence as exact model-visible
// tables. It returns only rows that fit the hard token budget, with each row
// still bound to the Citation source selected by the aggregate.
package qctx

import (
	"bytes"
	stdcontext "context"
	"encoding/csv"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	querybase "github.com/memoria-space/meking/query"
)

// TokenCounter counts the exact rendered text using the tokenizer already
// fixed by the owning Query aggregate's Corpora.
type TokenCounter interface {
	Count(ctx stdcontext.Context, text string) (int, error)
}

// ErrBudgetTooSmall reports that Prefix and the table header alone exceed the
// hard request budget, so no valid model-visible table can be produced.
var ErrBudgetTooSmall = errors.New("Query Context budget cannot contain its prefix and table header")

// Row is evidence already selected and ordered by a Query aggregate. qctx does
// not interpret its domain values or change that order; it only decides which
// prefix of rows fits the rendered-table budget.
type Row struct {
	// Values is the only model-visible input from this Row. It comes from the
	// aggregate, has one cell per Request.Columns entry, and excludes generated id.
	Values []string
	// CorporaID is never rendered. After the model answers, Citation audit uses
	// it with TextUnitIDs to select the exact Corpus publication for source reads.
	CorporaID string
	// TextUnitIDs is never rendered or read by qctx. It comes from the selected
	// evidence object; qctx copies, sorts, and deduplicates it so the aggregate
	// can resolve cited sources after the model answers.
	TextUnitIDs []string
}

// Request describes one ordered model table. The aggregate has already chosen
// the Dataset, candidate order, source identities, and total token allocation.
type Request struct {
	// Dataset is one supported model-visible citation table name.
	Dataset querybase.CitationDataset
	// Columns are non-empty, one-line payload column names; the Builder prepends id.
	Columns []string
	// Rows preserve aggregate-selected order and are considered exactly once.
	Rows []Row
	// Delimiter is one non-quote, non-newline CSV delimiter rune.
	Delimiter rune
	// MaxTokens is the positive hard budget for Prefix plus the complete table.
	MaxTokens int
	// Prefix is exact model-visible text placed before the CSV table. The caller
	// owns its structure, including any trailing separator or newline.
	Prefix string
	// StartRecordID is the non-negative ID assigned to the first accepted row;
	// later accepted rows increase it by one.
	StartRecordID int
}

// AcceptedRow keeps the rendered cells and their Citation scope together after
// budget admission. This prevents an aggregate from auditing a row that did
// not enter the Prompt or from attaching the visible row ID to different
// TextUnit sources.
type AcceptedRow struct {
	// Values is the unescaped cell sequence used to render this row, including
	// the generated `id` cell at index zero.
	Values []string
	// CitationRecord is built from the same assigned ID and source binding as
	// Values. The owning aggregate passes it to Citation audit after answering.
	CitationRecord querybase.CitationRecord
}

// Table is the immutable result of rendering one Request. Rows and Text are two
// views of the same admitted prefix and are copied away from caller input.
type Table struct {
	// Text is the only text whose token count and row admission are authoritative.
	Text string
	// TokenCount is measured from Text after CSV escaping, never estimated from
	// the unescaped cells.
	TokenCount int
	// Rows contains exactly the model-visible data rows, in rendered order.
	Rows []AcceptedRow
	// Truncated is true when the first excluded row exceeded MaxTokens; because
	// order is fixed, that row and every later candidate remain outside Text.
	Truncated bool
}

// Builder applies exact CSV rendering and full-row budget admission without
// sorting candidates or interpreting their domain values.
type Builder struct {
	// tokens is bound by the aggregate to the fixed Corpus tokenizer.
	tokens TokenCounter
}

// NewBuilder creates a table Builder with an explicit fixed-view token counter.
func NewBuilder(tokens TokenCounter) (*Builder, error) {
	if tokens == nil {
		return nil, errors.New("create Query Context Builder: TokenCounter is required")
	}
	return &Builder{tokens: tokens}, nil
}

// Build assigns row IDs only after exact rendered text fits the budget. A row
// is either included completely or excluded together with every later row.
func (b *Builder) Build(
	ctx stdcontext.Context,
	request Request,
) (Table, error) {
	if b == nil || b.tokens == nil {
		return Table{}, errors.New("Query Context Builder is not configured")
	}
	if err := validateRequest(request); err != nil {
		return Table{}, err
	}
	if err := ctx.Err(); err != nil {
		return Table{}, err
	}
	columns := append([]string{"id"}, request.Columns...)
	rows := make([]AcceptedRow, 0, len(request.Rows))
	text, err := render(request.Prefix, columns, rows, request.Delimiter)
	if err != nil {
		return Table{}, fmt.Errorf("render Query Context table header: %w", err)
	}
	tokenCount, err := b.tokens.Count(ctx, text)
	if err != nil {
		return Table{}, fmt.Errorf("count Query Context table header: %w", err)
	}
	if tokenCount < 0 {
		return Table{}, errors.New("count Query Context table header: TokenCounter returned a negative count")
	}
	if tokenCount > request.MaxTokens {
		return Table{}, ErrBudgetTooSmall
	}
	truncated := false
	for _, row := range request.Rows {
		if err := ctx.Err(); err != nil {
			return Table{}, err
		}
		accepted, err := makeAcceptedRow(request.Dataset, request.StartRecordID+len(rows), row)
		if err != nil {
			return Table{}, err
		}
		candidate := append(append([]AcceptedRow(nil), rows...), accepted)
		candidateText, err := render(request.Prefix, columns, candidate, request.Delimiter)
		if err != nil {
			return Table{}, fmt.Errorf("render Query Context table row: %w", err)
		}
		candidateTokens, err := b.tokens.Count(ctx, candidateText)
		if err != nil {
			return Table{}, fmt.Errorf("count Query Context table row: %w", err)
		}
		if candidateTokens < 0 {
			return Table{}, errors.New("count Query Context table row: TokenCounter returned a negative count")
		}
		if candidateTokens > request.MaxTokens {
			truncated = true
			break
		}
		rows = candidate
		text = candidateText
		tokenCount = candidateTokens
	}
	return Table{
		Text:       text,
		TokenCount: tokenCount,
		Rows:       copyAcceptedRows(rows),
		Truncated:  truncated,
	}, nil
}

func validateRequest(request Request) error {
	if !supportedDataset(request.Dataset) {
		return errors.New("Query Context Dataset is unsupported")
	}
	if len(request.Columns) == 0 {
		return errors.New("Query Context table requires payload columns")
	}
	seenColumns := map[string]struct{}{"id": {}}
	for _, column := range request.Columns {
		if strings.TrimSpace(column) == "" || strings.ContainsAny(column, "\r\n") {
			return errors.New("Query Context column names must be non-empty and single-line")
		}
		if _, duplicate := seenColumns[column]; duplicate {
			return fmt.Errorf("Query Context column %q conflicts with another column", column)
		}
		seenColumns[column] = struct{}{}
	}
	if request.Delimiter == 0 || request.Delimiter == '"' ||
		request.Delimiter == '\r' || request.Delimiter == '\n' ||
		request.Delimiter == utf8.RuneError {
		return errors.New("Query Context delimiter must be one non-quote line rune")
	}
	if request.MaxTokens <= 0 {
		return errors.New("Query Context token budget must be positive")
	}
	if request.StartRecordID < 0 {
		return errors.New("Query Context starting record ID must be non-negative")
	}
	for index, row := range request.Rows {
		if len(row.Values) != len(request.Columns) {
			return fmt.Errorf(
				"Query Context row %d has %d values for %d columns",
				index,
				len(row.Values),
				len(request.Columns),
			)
		}
		if strings.TrimSpace(row.CorporaID) == "" || row.CorporaID != strings.TrimSpace(row.CorporaID) {
			return fmt.Errorf(
				"Query Context row %d CorporaID is required without surrounding whitespace",
				index,
			)
		}
		if _, err := normalizeTextUnitIDs(row.TextUnitIDs); err != nil {
			return fmt.Errorf("Query Context row %d: %w", index, err)
		}
	}
	return nil
}

func makeAcceptedRow(
	dataset querybase.CitationDataset,
	recordID int,
	row Row,
) (AcceptedRow, error) {
	if strings.TrimSpace(row.CorporaID) == "" || row.CorporaID != strings.TrimSpace(row.CorporaID) {
		return AcceptedRow{}, errors.New("Query Context row CorporaID is required without surrounding whitespace")
	}
	textUnitIDs, err := normalizeTextUnitIDs(row.TextUnitIDs)
	if err != nil {
		return AcceptedRow{}, err
	}
	values := make([]string, 0, len(row.Values)+1)
	values = append(values, strconv.Itoa(recordID))
	values = append(values, row.Values...)
	return AcceptedRow{
		Values: append([]string(nil), values...),
		CitationRecord: querybase.CitationRecord{
			Reference:   querybase.CitationReference{Dataset: dataset, RecordID: recordID},
			CorporaID:   row.CorporaID,
			TextUnitIDs: textUnitIDs,
		},
	}, nil
}

func normalizeTextUnitIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, errors.New("Query Context row TextUnitIDs are required")
	}
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return nil, errors.New("Query Context TextUnitID is empty or has surrounding whitespace")
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

func render(prefix string, columns []string, rows []AcceptedRow, delimiter rune) (string, error) {
	var buffer bytes.Buffer
	buffer.WriteString(prefix)
	writer := csv.NewWriter(&buffer)
	writer.Comma = delimiter
	if err := writer.Write(columns); err != nil {
		return "", err
	}
	for _, row := range rows {
		if err := writer.Write(row.Values); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func copyAcceptedRows(rows []AcceptedRow) []AcceptedRow {
	result := make([]AcceptedRow, len(rows))
	for index, row := range rows {
		row.Values = append([]string(nil), row.Values...)
		row.CitationRecord.TextUnitIDs = append(
			[]string(nil),
			row.CitationRecord.TextUnitIDs...,
		)
		result[index] = row
	}
	return result
}
