package local

import (
	"context"
	"testing"

	querybase "github.com/memoria-space/meking/query"
	"github.com/memoria-space/meking/query/qctx"
)

type runeTokenCounter struct{}

func (runeTokenCounter) Count(_ context.Context, _ string, text string) (int, error) {
	return len([]rune(text)), nil
}

func TestAppendTableReportsTruncationFromTotalContextBudget(t *testing.T) {
	rows := []qctx.Row{
		{Values: []string{"first"}, CorporaID: "corpus", TextUnitIDs: []string{"unit-1"}},
		{Values: []string{"second row is deliberately longer"}, CorporaID: "corpus", TextUnitIDs: []string{"unit-2"}},
	}
	counter := runeTokenCounter{}
	builder, err := qctx.NewBuilder(fixedTokenCounter{counter: counter, corporaID: "corpus"})
	if err != nil {
		t.Fatalf("NewBuilder() error = %v", err)
	}
	oneRow, err := builder.Build(t.Context(), qctx.Request{
		Dataset: querybase.CitationEntities, Columns: []string{"entity"},
		Rows: rows[:1], Delimiter: '|', MaxTokens: 1000, Prefix: "-----Entities-----\n",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	current := "conversation"
	maxTokens := len([]rune(joinContext(current, oneRow.Text)))
	searcher := &Searcher{tokens: counter, config: Config{MaxContextTokens: maxTokens}}

	table, text, err := searcher.appendTable(t.Context(), builder, current, qctx.Request{
		Dataset: querybase.CitationEntities, Columns: []string{"entity"},
		Rows: rows, Delimiter: '|', MaxTokens: 1000, Prefix: "-----Entities-----\n",
	})
	if err != nil {
		t.Fatalf("appendTable() error = %v", err)
	}
	if len(table.Rows) != 1 || !table.Truncated {
		t.Fatalf("table = %#v, want one row and Truncated", table)
	}
	if text != joinContext(current, oneRow.Text) {
		t.Fatalf("text = %q, want %q", text, joinContext(current, oneRow.Text))
	}
}
