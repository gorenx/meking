package global

import (
	"context"
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
)

// conversationExchange groups one user turn with the following non-user turns
// so the configured history policy can retain or omit answers as one unit.
type conversationExchange struct {
	// user starts this exchange and always contributes one rendered row.
	user querybase.ConversationTurn
	// answers preserve the following assistant or system turn order until the next user.
	answers []querybase.ConversationTurn
}

func (b *ContextBuilder) buildConversation(
	ctx context.Context,
	turns []querybase.ConversationTurn,
	config ContextConfig,
	corporaID string,
) (string, querybase.ContextSection, error) {
	exchanges := groupConversation(turns)
	if len(exchanges) > config.ConversationHistoryTurns {
		exchanges = exchanges[:config.ConversationHistoryTurns]
	}
	section := querybase.ContextSection{Name: "conversation history"}
	if len(exchanges) == 0 {
		return "", section, nil
	}

	accepted := make([][]string, 0, len(exchanges)*2)
	for _, exchange := range exchanges {
		candidate := copyRows(accepted)
		candidate = append(candidate, []string{string(querybase.RoleUser), exchange.user.Content})
		if !config.ConversationUserTurnsOnly && len(exchange.answers) > 0 {
			answers := make([]string, len(exchange.answers))
			for index := range exchange.answers {
				answers[index] = exchange.answers[index].Content
			}
			candidate = append(candidate, []string{
				string(querybase.RoleAssistant),
				strings.Join(answers, "\n"),
			})
		}
		text, err := renderNamedTable(
			"Conversation History",
			[]string{"turn", "content"},
			candidate,
			config.ColumnDelimiter,
		)
		if err != nil {
			return "", querybase.ContextSection{}, fmt.Errorf("render Global conversation: %w", err)
		}
		count, err := b.tokens.Count(ctx, corporaID, text)
		if err != nil {
			return "", querybase.ContextSection{}, fmt.Errorf("count Global conversation: %w", err)
		}
		if count > config.MaxContextTokens {
			break
		}
		accepted = candidate
	}

	if len(accepted) == 0 {
		return "-----Conversation History-----\n", section, nil
	}
	text, err := renderNamedTable(
		"Conversation History",
		[]string{"turn", "content"},
		accepted,
		config.ColumnDelimiter,
	)
	if err != nil {
		return "", querybase.ContextSection{}, fmt.Errorf("render Global conversation: %w", err)
	}
	section.Columns = []string{"turn", "content"}
	section.Rows = tableRows(accepted, false)
	return text, section, nil
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

func renderNamedTable(
	name string,
	columns []string,
	rows [][]string,
	delimiter string,
) (string, error) {
	table, err := renderTable(columns, rows, delimiter)
	if err != nil {
		return "", err
	}
	return "-----" + name + "-----\n" + table, nil
}

func copyRows(rows [][]string) [][]string {
	result := make([][]string, len(rows))
	for index, row := range rows {
		result[index] = append([]string(nil), row...)
	}
	return result
}
