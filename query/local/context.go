package local

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/qctx"
	queryreport "github.com/memoria-space/meking/query/report"
	querysource "github.com/memoria-space/meking/query/source"
)

func (s *Searcher) buildContext(
	ctx context.Context,
	view queryreport.View,
	conversation []querybase.ConversationTurn,
	evidence localEvidence,
) (Context, error) {
	history, historyTokens, err := s.buildConversation(ctx, view.CorporaID, conversation)
	if err != nil {
		return Context{}, err
	}
	remaining := s.config.MaxContextTokens - historyTokens
	if remaining <= 0 {
		return Context{}, querybase.NewBudgetExceededFailure(nil)
	}
	reportBudget := int(float64(remaining) * s.config.CommunityProportion)
	sourceBudget := int(float64(remaining) * s.config.TextUnitProportion)
	graphBudget := remaining - reportBudget - sourceBudget

	result := Context{Conversation: history, Text: history}
	builder, err := qctx.NewBuilder(fixedTokenCounter{
		counter: s.tokens, corporaID: view.CorporaID,
	})
	if err != nil {
		return Context{}, querybase.NewInternalFailure(err)
	}

	result.Reports, result.Text, err = s.appendTable(ctx, builder, result.Text, qctx.Request{
		Dataset: querybase.CitationReports, Columns: []string{"title", "content"},
		Rows: reportRows(view.CorporaID, evidence.reports), Delimiter: s.config.Delimiter,
		MaxTokens: reportBudget, Prefix: "-----Reports-----\n",
	})
	if err != nil {
		return Context{}, err
	}

	result.Entities, result.Text, err = s.appendTable(ctx, builder, result.Text, qctx.Request{
		Dataset: querybase.CitationEntities,
		Columns: []string{"entity", "description", "number of relationships"},
		Rows:    entityRows(view.CorporaID, evidence.selectedEntities), Delimiter: s.config.Delimiter,
		MaxTokens: graphBudget, Prefix: "-----Entities-----\n",
	})
	if err != nil {
		return Context{}, err
	}
	graphBudget = max(0, graphBudget-result.Entities.TokenCount)
	acceptedEntityIDs := acceptedIDs(evidence.selectedEntities, result.Entities)
	relationships := relationshipsForAcceptedEntities(evidence.relationships, acceptedEntityIDs)
	titles := endpointTitles(evidence.selectedEntities, evidence.reportEntities)
	result.Relationships, result.Text, err = s.appendTable(ctx, builder, result.Text, qctx.Request{
		Dataset:   querybase.CitationRelationships,
		Columns:   []string{"source", "target", "description", "weight"},
		Rows:      relationshipRows(view.CorporaID, relationships, titles),
		Delimiter: s.config.Delimiter, MaxTokens: graphBudget,
		Prefix: "-----Relationships-----\n",
	})
	if err != nil {
		return Context{}, err
	}
	graphBudget = max(0, graphBudget-result.Relationships.TokenCount)
	acceptedRelationshipIDs := acceptedIDs(relationships, result.Relationships)
	claims := claimsForAcceptedSubjects(evidence.claims, acceptedEntityIDs, acceptedRelationshipIDs)
	result.Claims, result.Text, err = s.appendTable(ctx, builder, result.Text, qctx.Request{
		Dataset: querybase.CitationClaims,
		Columns: []string{
			"subject", "type", "status", "start_date", "end_date", "description", "source_text",
		},
		Rows: claimRows(view.CorporaID, claims, titles), Delimiter: s.config.Delimiter,
		MaxTokens: graphBudget, Prefix: "-----Claims-----\n",
	})
	if err != nil {
		return Context{}, err
	}

	parentRecords := result.CitationRecords()
	sourceRows, err := s.sourceRows(ctx, view.CorporaID, parentRecords)
	if err != nil {
		return Context{}, err
	}
	result.Sources, result.Text, err = s.appendTable(ctx, builder, result.Text, qctx.Request{
		Dataset: querybase.CitationSources, Columns: []string{"text"}, Rows: sourceRows,
		Delimiter: s.config.Delimiter, MaxTokens: sourceBudget,
		Prefix: "-----Sources-----\n",
	})
	if err != nil {
		return Context{}, err
	}
	count, err := s.tokens.Count(ctx, view.CorporaID, result.Text)
	if err != nil {
		return Context{}, normalizeFailure(err)
	}
	if count < 0 {
		return Context{}, querybase.NewInternalFailure(
			errors.New("Local TokenCounter returned a negative final count"),
		)
	}
	if count > s.config.MaxContextTokens {
		return Context{}, querybase.NewInternalFailure(fmt.Errorf(
			"Local context uses %d tokens after bounded admission of %d", count, s.config.MaxContextTokens,
		))
	}
	return result, nil
}

func (s *Searcher) appendTable(
	ctx context.Context,
	builder *qctx.Builder,
	current string,
	request qctx.Request,
) (qctx.Table, string, error) {
	if request.MaxTokens <= 0 || len(request.Rows) == 0 {
		return qctx.Table{}, current, nil
	}
	table, err := builder.Build(ctx, request)
	if errors.Is(err, qctx.ErrBudgetTooSmall) {
		return qctx.Table{}, current, nil
	}
	if err != nil {
		return qctx.Table{}, current, normalizePublicationFailure(err)
	}
	trimmedForTotalBudget := false
	for len(table.Rows) > 0 {
		candidate := joinContext(current, table.Text)
		count, countErr := s.tokens.Count(ctx, table.Rows[0].CitationRecord.CorporaID, candidate)
		if countErr != nil {
			return qctx.Table{}, current, normalizeFailure(countErr)
		}
		if count < 0 {
			return qctx.Table{}, current, querybase.NewInternalFailure(
				errors.New("Local TokenCounter returned a negative count"),
			)
		}
		if count <= s.config.MaxContextTokens {
			if trimmedForTotalBudget {
				table.Truncated = true
			}
			return table, candidate, nil
		}
		trimmedForTotalBudget = true
		request.Rows = request.Rows[:len(table.Rows)-1]
		if len(request.Rows) == 0 {
			break
		}
		table, err = builder.Build(ctx, request)
		if err != nil {
			return qctx.Table{}, current, normalizePublicationFailure(err)
		}
	}
	return qctx.Table{}, current, nil
}

func reportRows(corporaID string, reports []rankedReport) []qctx.Row {
	rows := make([]qctx.Row, len(reports))
	for index, current := range reports {
		rows[index] = qctx.Row{
			Values:      []string{current.report.Title, current.report.FullContent},
			CorporaID:   corporaID,
			TextUnitIDs: append([]string(nil), current.report.Sources.TextUnitIDs...),
		}
	}
	return rows
}

func entityRows(corporaID string, entities []Entity) []qctx.Row {
	rows := make([]qctx.Row, len(entities))
	for index, entity := range entities {
		rows[index] = qctx.Row{
			Values:      []string{entity.Title, entity.Description, strconv.Itoa(entity.Degree)},
			CorporaID:   corporaID,
			TextUnitIDs: append([]string(nil), entity.TextUnitIDs...),
		}
	}
	return rows
}

func relationshipRows(
	corporaID string,
	relationships []Relationship,
	titles map[string]string,
) []qctx.Row {
	rows := make([]qctx.Row, len(relationships))
	for index, relationship := range relationships {
		source := titles[relationship.SourceEntityID]
		if source == "" {
			source = relationship.SourceEntityID
		}
		target := titles[relationship.TargetEntityID]
		if target == "" {
			target = relationship.TargetEntityID
		}
		rows[index] = qctx.Row{
			Values:      []string{source, target, relationship.Description, strconv.FormatFloat(relationship.Weight, 'g', -1, 64)},
			CorporaID:   corporaID,
			TextUnitIDs: append([]string(nil), relationship.TextUnitIDs...),
		}
	}
	return rows
}

func claimRows(corporaID string, claims []Claim, titles map[string]string) []qctx.Row {
	rows := make([]qctx.Row, len(claims))
	for index, claim := range claims {
		subject := claim.Subject.SubjectID()
		if entity, ok := claim.Subject.(EntityClaimSubject); ok && titles[entity.ID] != "" {
			subject = titles[entity.ID]
		}
		rows[index] = qctx.Row{
			Values: []string{
				subject, claim.Type, claim.Status, claim.StartDate, claim.EndDate,
				claim.Description, claim.SourceText,
			},
			CorporaID: corporaID, TextUnitIDs: []string{claim.TextUnitID},
		}
	}
	return rows
}

func (s *Searcher) sourceRows(
	ctx context.Context,
	corporaID string,
	records []querybase.CitationRecord,
) ([]qctx.Row, error) {
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	for _, record := range records {
		for _, id := range record.TextUnitIDs {
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	sources, err := s.sources.Read(ctx, corporaID, append([]string(nil), ids...))
	if err != nil {
		return nil, normalizePublicationFailure(err)
	}
	byID := make(map[string]querysource.TextUnitSource, len(ids))
	for _, source := range sources {
		if source.CorporaID != corporaID {
			continue
		}
		if _, requested := seen[source.TextUnitID]; !requested {
			continue
		}
		if _, exists := byID[source.TextUnitID]; !exists {
			byID[source.TextUnitID] = source
		}
	}
	rows := make([]qctx.Row, 0, len(ids))
	for _, id := range ids {
		source, exists := byID[id]
		if !exists {
			continue
		}
		rows = append(rows, qctx.Row{
			Values: []string{source.Text}, CorporaID: corporaID, TextUnitIDs: []string{id},
		})
	}
	return rows, nil
}

func acceptedIDs[T interface{ recordID() string }](values []T, table qctx.Table) map[string]struct{} {
	count := min(len(values), len(table.Rows))
	result := make(map[string]struct{}, count)
	for index := range count {
		result[values[index].recordID()] = struct{}{}
	}
	return result
}

func (e Entity) recordID() string       { return e.ID }
func (r Relationship) recordID() string { return r.ID }

func relationshipsForAcceptedEntities(
	values []Relationship,
	accepted map[string]struct{},
) []Relationship {
	result := make([]Relationship, 0, len(values))
	for _, relationship := range values {
		_, source := accepted[relationship.SourceEntityID]
		_, target := accepted[relationship.TargetEntityID]
		if source || target {
			result = append(result, relationship)
		}
	}
	return result
}

func claimsForAcceptedSubjects(
	values []Claim,
	entities map[string]struct{},
	relationships map[string]struct{},
) []Claim {
	result := make([]Claim, 0, len(values))
	for _, claim := range values {
		include := false
		switch subject := claim.Subject.(type) {
		case EntityClaimSubject:
			_, include = entities[subject.ID]
		case RelationClaimSubject:
			_, include = relationships[subject.ID]
		}
		if include {
			result = append(result, claim)
		}
	}
	return result
}

func endpointTitles(selected []Entity, reports []Entity) map[string]string {
	result := make(map[string]string, len(selected)+len(reports))
	for _, entity := range reports {
		if _, exists := result[entity.ID]; !exists {
			result[entity.ID] = entity.Title
		}
	}
	for _, entity := range selected {
		if _, exists := result[entity.ID]; !exists {
			result[entity.ID] = entity.Title
		}
	}
	return result
}

// CitationRecords returns the exact row scope represented by this model-visible
// Context. DRIFT uses it to audit a branch before assigning request-global IDs.
func (value Context) CitationRecords() []querybase.CitationRecord {
	tables := []qctx.Table{
		value.Reports, value.Entities, value.Relationships, value.Claims, value.Sources,
	}
	result := make([]querybase.CitationRecord, 0)
	for _, table := range tables {
		for _, row := range table.Rows {
			record := row.CitationRecord
			record.TextUnitIDs = append([]string(nil), record.TextUnitIDs...)
			result = append(result, record)
		}
	}
	return result
}

func contextRecordCount(value Context) int {
	return len(value.Reports.Rows) + len(value.Entities.Rows) +
		len(value.Relationships.Rows) + len(value.Claims.Rows) + len(value.Sources.Rows)
}

func joinContext(current, section string) string {
	if current == "" {
		return section
	}
	if section == "" {
		return current
	}
	return current + "\n\n" + section
}

// fixedTokenCounter binds qctx's storage-neutral counter contract to the one
// Corpora tokenizer fixed by the Local request.
type fixedTokenCounter struct {
	// counter resolves the tokenizer for corporaID without exposing it to qctx.
	counter TokenCounter
	// corporaID is the immutable evidence publication fixed by Local.
	corporaID string
}

func (c fixedTokenCounter) Count(ctx context.Context, text string) (int, error) {
	return c.counter.Count(ctx, c.corporaID, text)
}

// conversationExchange groups one user question with the following non-user
// turns so Local can retain complete recent exchanges without mixing questions.
type conversationExchange struct {
	user    querybase.ConversationTurn
	answers []querybase.ConversationTurn
}

func (s *Searcher) buildConversation(
	ctx context.Context,
	corporaID string,
	turns []querybase.ConversationTurn,
) (string, int, error) {
	exchanges := groupConversation(turns)
	if len(exchanges) > s.config.ConversationTurns {
		exchanges = exchanges[len(exchanges)-s.config.ConversationTurns:]
	}
	if len(exchanges) == 0 {
		return "", 0, nil
	}
	rows := make([][]string, 0, len(exchanges)*2)
	text := ""
	count := 0
	for _, exchange := range exchanges {
		candidate := copyStringRows(rows)
		candidate = append(candidate, []string{string(querybase.RoleUser), exchange.user.Content})
		if !s.config.ConversationUserTurnsOnly && len(exchange.answers) > 0 {
			answers := make([]string, len(exchange.answers))
			for index, answer := range exchange.answers {
				answers[index] = answer.Content
			}
			candidate = append(candidate, []string{string(querybase.RoleAssistant), strings.Join(answers, "\n")})
		}
		candidateText, err := renderConversationTable(candidate, s.config.Delimiter)
		if err != nil {
			return "", 0, querybase.NewInternalFailure(err)
		}
		candidateCount, err := s.tokens.Count(ctx, corporaID, candidateText)
		if err != nil {
			return "", 0, normalizeFailure(err)
		}
		if candidateCount < 0 {
			return "", 0, querybase.NewInternalFailure(
				errors.New("Local TokenCounter returned a negative conversation count"),
			)
		}
		if candidateCount > s.config.MaxContextTokens {
			break
		}
		rows, text, count = candidate, candidateText, candidateCount
	}
	if len(rows) == 0 {
		return "", 0, querybase.NewBudgetExceededFailure(nil)
	}
	return text, count, nil
}

func groupConversation(turns []querybase.ConversationTurn) []conversationExchange {
	result := make([]conversationExchange, 0)
	var current *conversationExchange
	for _, turn := range turns {
		if turn.Role == querybase.RoleUser {
			if current != nil {
				result = append(result, *current)
			}
			current = &conversationExchange{user: turn}
			continue
		}
		if current != nil {
			current.answers = append(current.answers, turn)
		}
	}
	if current != nil {
		result = append(result, *current)
	}
	return result
}

func renderConversationTable(rows [][]string, delimiter rune) (string, error) {
	var buffer bytes.Buffer
	buffer.WriteString("-----Conversation History-----\n")
	writer := csv.NewWriter(&buffer)
	writer.Comma = delimiter
	if err := writer.Write([]string{"turn", "content"}); err != nil {
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

func copyStringRows(rows [][]string) [][]string {
	result := make([][]string, len(rows))
	for index, row := range rows {
		result[index] = append([]string(nil), row...)
	}
	return result
}
