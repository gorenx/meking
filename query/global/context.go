package global

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/internal/deterministic"
	queryreport "github.com/memoria-space/meking/query/report"
)

// ContextBuilder converts fixed ReportSet evidence into deterministic,
// token-bounded Map inputs without opening storage or resolving current state.
type ContextBuilder struct {
	// tokens applies the configured budgets to the exact rendered prompt text.
	tokens TokenCounter
	// selector is present only when the composition root enables dynamic selection.
	selector CommunitySelector
}

// NewContextBuilder creates the provider-independent Global context builder.
func NewContextBuilder(
	tokens TokenCounter,
	selector CommunitySelector,
) (*ContextBuilder, error) {
	if tokens == nil {
		return nil, errors.New("create Global context builder: TokenCounter is required")
	}
	return &ContextBuilder{tokens: tokens, selector: selector}, nil
}

// Build selects Reports and creates the exact evidence chunks supplied to Map.
func (b *ContextBuilder) Build(
	ctx context.Context,
	request ContextRequest,
) (Context, error) {
	if b == nil || b.tokens == nil {
		return Context{}, errors.New("Global context builder is not configured")
	}
	if err := validateContextRequest(request); err != nil {
		return Context{}, err
	}
	if err := ctx.Err(); err != nil {
		return Context{}, err
	}
	corporaID := request.Evidence.ReportView.CorporaID
	history, historySection, err := b.buildConversation(
		ctx,
		request.Conversation,
		request.Config,
		corporaID,
	)
	if err != nil {
		return Context{}, err
	}
	reports, err := b.selectReports(ctx, request)
	if err != nil {
		return Context{}, err
	}
	for index := range reports {
		reports[index].recordID = index
	}
	if request.Config.ShuffleData {
		shuffleReports(reports, request.Config.RandomSeed)
	}
	chunks, accepted, reportSection, err := b.buildReportBatches(
		ctx,
		reports,
		history,
		request.Config,
		corporaID,
	)
	if err != nil {
		return Context{}, err
	}
	sections := make([]querybase.ContextSection, 0, 2)
	if len(historySection.Rows) > 0 {
		sections = append(sections, historySection)
	}
	if len(reportSection.Rows) > 0 {
		sections = append(sections, reportSection)
	}
	view := request.Evidence.ReportView
	return Context{
		EpochID:     view.EpochID,
		ReportSetID: view.ReportSetID, CommunitySetID: view.CommunitySetID,
		CorporaID: view.CorporaID,
		Chunks:    chunks, Reports: accepted, Sections: sections,
	}, nil
}

// weightedReport retains one selected Report, its hierarchy node, its
// occurrence weight, and the request-local row identity assigned before batching.
type weightedReport struct {
	// recordID is a zero-based request-local prompt row identity.
	recordID int
	// report is the immutable model evidence selected from the fixed ReportSet.
	report queryreport.PublishedReport
	// community supplies the stable hierarchy identity summarized by report.
	community queryreport.PublishedCommunity
	// weight is the unique Entity TextUnit occurrence count, optionally normalized.
	weight float64
}

func (b *ContextBuilder) selectReports(
	ctx context.Context,
	request ContextRequest,
) ([]weightedReport, error) {
	view := request.Evidence.ReportView
	candidates := make([]ReportCandidate, 0, len(view.Reports))
	for index, report := range view.Reports {
		community := view.Communities[index]
		if request.Config.CommunityLevel != nil && community.Level > *request.Config.CommunityLevel {
			continue
		}
		candidates = append(candidates, ReportCandidate{Report: report, Community: community})
	}
	selected := candidates
	if request.Config.DynamicSelection {
		if b.selector == nil {
			return nil, errors.New("Global dynamic Community selection is not configured")
		}
		ids, err := b.selector.SelectCommunities(ctx, SelectionRequest{
			Question: request.Question,
			Reports:  copyReportCandidates(candidates),
		})
		if err != nil {
			return nil, fmt.Errorf("select Global Communities: %w", err)
		}
		selected = selectedCandidates(candidates, ids)
	}
	selected = reportsMeetingMinimumRank(selected, request.Config.MinimumCommunityRank)
	weights := reportWeights(selected, request.Evidence.Entities, request.Config)
	result := make([]weightedReport, len(selected))
	for index, candidate := range selected {
		result[index] = weightedReport{
			report: candidate.Report, community: candidate.Community,
			weight: weights[candidate.Report.ID],
		}
	}
	return result, nil
}

func reportsMeetingMinimumRank(
	candidates []ReportCandidate,
	minimum float64,
) []ReportCandidate {
	selected := make([]ReportCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Report.Rank >= minimum {
			selected = append(selected, candidate)
		}
	}
	return selected
}

func selectedCandidates(
	candidates []ReportCandidate,
	ids []string,
) []ReportCandidate {
	byCommunity := make(map[string]ReportCandidate, len(candidates))
	for _, candidate := range candidates {
		byCommunity[candidate.Community.ID] = candidate
	}
	selected := make([]ReportCandidate, 0, len(ids))
	for _, id := range ids {
		selected = append(selected, byCommunity[id])
	}
	return selected
}

func reportWeights(
	reports []ReportCandidate,
	entities []EntityEvidence,
	config ContextConfig,
) map[string]float64 {
	result := make(map[string]float64, len(reports))
	if !config.IncludeCommunityWeight {
		return result
	}
	byReference := make(map[querybase.KnowledgeReference][]string, len(entities))
	for _, entity := range entities {
		byReference[entity.Reference] = entity.TextUnitIDs
	}
	maximum := 0.0
	for _, candidate := range reports {
		textUnits := make(map[string]struct{})
		for _, reference := range candidate.Report.Sources.Entities {
			for _, id := range byReference[reference] {
				textUnits[id] = struct{}{}
			}
		}
		weight := float64(len(textUnits))
		result[candidate.Report.ID] = weight
		if weight > maximum {
			maximum = weight
		}
	}
	if config.NormalizeCommunityWeight && len(reports) > 0 {
		if maximum > 0 {
			for id, weight := range result {
				result[id] = weight / maximum
			}
		}
	}
	return result
}

func copyReportCandidates(values []ReportCandidate) []ReportCandidate {
	result := make([]ReportCandidate, len(values))
	for index, value := range values {
		value.Report.Findings = append([]queryreport.ReportFinding(nil), value.Report.Findings...)
		value.Report.Sources.Entities = append(
			[]querybase.KnowledgeReference(nil),
			value.Report.Sources.Entities...,
		)
		value.Report.Sources.Relations = append(
			[]querybase.KnowledgeReference(nil),
			value.Report.Sources.Relations...,
		)
		value.Report.Sources.Claims = append(
			[]querybase.ClaimReference(nil),
			value.Report.Sources.Claims...,
		)
		value.Report.Sources.TextUnitIDs = append([]string(nil), value.Report.Sources.TextUnitIDs...)
		value.Community.EntityIDs = append([]string(nil), value.Community.EntityIDs...)
		if value.Community.ParentID != nil {
			parent := *value.Community.ParentID
			value.Community.ParentID = &parent
		}
		result[index] = value
	}
	return result
}

func shuffleReports(reports []weightedReport, seed uint64) {
	deterministic.Shuffle(reports, seed)
}

// reportRow keeps the selected identity and its rendered values together while
// token batches are sorted, so diagnostics cannot drift from model-visible rows.
type reportRow struct {
	// report identifies and ranks the immutable evidence represented by values.
	report weightedReport
	// values is the complete escaped-table input in the configured column order.
	values []string
}

func (b *ContextBuilder) buildReportBatches(
	ctx context.Context,
	reports []weightedReport,
	history string,
	config ContextConfig,
	corporaID string,
) ([]ContextChunk, []ReportReference, querybase.ContextSection, error) {
	columns := []string{"id", "title"}
	if config.IncludeCommunityWeight {
		columns = append(columns, "occurrence weight")
	}
	if config.UseCommunitySummary {
		columns = append(columns, "summary")
	} else {
		columns = append(columns, "content")
	}
	if config.IncludeCommunityRank {
		columns = append(columns, "rank")
	}
	section := querybase.ContextSection{
		Name: strings.ToLower(config.ContextName), Columns: append([]string(nil), columns...),
	}
	if len(reports) == 0 {
		return nil, nil, section, nil
	}
	header := "-----" + config.ContextName + "-----\n" +
		strings.Join(columns, config.ColumnDelimiter) + "\n"
	headerTokens, err := b.tokens.Count(ctx, corporaID, header)
	if err != nil {
		return nil, nil, querybase.ContextSection{}, fmt.Errorf("count Global Report header: %w", err)
	}
	currentTokens := headerTokens
	current := make([]reportRow, 0)
	batches := make([][]reportRow, 0)
	for _, candidate := range reports {
		if err := ctx.Err(); err != nil {
			return nil, nil, querybase.ContextSection{}, err
		}
		content := candidate.report.FullContent
		if config.UseCommunitySummary {
			content = candidate.report.Summary
		}
		values := []string{strconv.Itoa(candidate.recordID), candidate.report.Title}
		if config.IncludeCommunityWeight {
			values = append(values, formatFloat(candidate.weight))
		}
		values = append(values, content)
		if config.IncludeCommunityRank {
			values = append(values, formatFloat(candidate.report.Rank))
		}
		rowTokens, err := b.tokens.Count(
			ctx,
			corporaID,
			strings.Join(values, config.ColumnDelimiter)+"\n",
		)
		if err != nil {
			return nil, nil, querybase.ContextSection{}, fmt.Errorf("count Global Report row: %w", err)
		}
		if currentTokens+rowTokens > config.MaxContextTokens {
			if len(current) > 0 {
				batches = append(batches, current)
			}
			current = make([]reportRow, 0)
			currentTokens = headerTokens
		}
		current = append(current, reportRow{report: candidate, values: values})
		currentTokens += rowTokens
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}

	chunks := make([]ContextChunk, 0, len(batches))
	accepted := make([]ReportReference, 0, len(reports))
	for batchIndex, batch := range batches {
		sort.SliceStable(batch, func(left, right int) bool {
			if batch[left].report.weight != batch[right].report.weight {
				return batch[left].report.weight > batch[right].report.weight
			}
			return batch[left].report.report.Rank > batch[right].report.report.Rank
		})
		rows := make([][]string, len(batch))
		reportIDs := make([]int, len(batch))
		for index, row := range batch {
			rows[index] = append([]string(nil), row.values...)
			reportIDs[index] = row.report.recordID
			accepted = append(accepted, ReportReference{
				RecordID:    row.report.recordID,
				ReportID:    row.report.report.ID,
				CommunityID: row.report.community.ID,
				TextUnitIDs: append(
					[]string(nil),
					row.report.report.Sources.TextUnitIDs...,
				),
			})
			section.Rows = append(section.Rows, querybase.ContextRow{
				Values: append([]string(nil), row.values...), InContext: true,
			})
		}
		text, err := renderTable(columns, rows, config.ColumnDelimiter)
		if err != nil {
			return nil, nil, querybase.ContextSection{}, fmt.Errorf("render Global Report batch: %w", err)
		}
		if history != "" {
			text = history + "\n\n" + text
		}
		tokenCount, err := b.tokens.Count(ctx, corporaID, text)
		if err != nil {
			return nil, nil, querybase.ContextSection{}, fmt.Errorf("count Global context chunk: %w", err)
		}
		chunks = append(chunks, ContextChunk{
			Index: batchIndex, Text: text, TokenCount: tokenCount,
			Columns: append([]string(nil), columns...), Rows: tableRows(rows, true),
			ReportIDs: reportIDs,
		})
	}
	return chunks, accepted, section, nil
}

func renderTable(columns []string, rows [][]string, delimiter string) (string, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.Comma = []rune(delimiter)[0]
	if err := writer.Write(columns); err != nil {
		return "", err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func formatFloat(value float64) string {
	result := strconv.FormatFloat(value, 'f', -1, 64)
	if !strings.ContainsAny(result, ".eE") {
		result += ".0"
	}
	return result
}

func tableRows(rows [][]string, inContext bool) []querybase.ContextRow {
	result := make([]querybase.ContextRow, len(rows))
	for index, row := range rows {
		result[index] = querybase.ContextRow{
			Values: append([]string(nil), row...), InContext: inContext,
		}
	}
	return result
}
