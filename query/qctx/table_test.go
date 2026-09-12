package qctx

import (
	stdcontext "context"
	"errors"
	"reflect"
	"testing"
	"unicode/utf8"

	querybase "github.com/memoria-space/meking/query"
)

type codePointCounter struct{}

func (codePointCounter) Count(_ stdcontext.Context, text string) (int, error) {
	return utf8.RuneCountInString(text), nil
}

func TestBuilderBudgetsExactEscapedRowsAndBindsCitationScope(t *testing.T) {
	builder, err := NewBuilder(codePointCounter{})
	if err != nil {
		t.Fatalf("NewBuilder() error = %v", err)
	}
	first := Row{
		Values: []string{"comma, quote\" and\nnewline"}, CorporaID: "corpus",
		TextUnitIDs: []string{"unit-b", "unit-a", "unit-b"},
	}
	one, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationSources, Columns: []string{"text"}, Rows: []Row{first},
		Delimiter: '|', MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("Build one row: %v", err)
	}
	table, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationSources, Columns: []string{"text"},
		Rows: []Row{
			first,
			{Values: []string{"second"}, CorporaID: "corpus", TextUnitIDs: []string{"unit-c"}},
		},
		Delimiter: '|', MaxTokens: one.TokenCount,
	})
	if err != nil {
		t.Fatalf("Build budgeted table: %v", err)
	}
	if len(table.Rows) != 1 || !table.Truncated || table.TokenCount != one.TokenCount ||
		table.Rows[0].CitationRecord.Reference.RecordID != 0 ||
		!reflect.DeepEqual(table.Rows[0].CitationRecord.TextUnitIDs, []string{"unit-a", "unit-b"}) ||
		table.Text != one.Text {
		t.Fatalf("table = %#v", table)
	}
	if table.Text != "id|text\n0|\"comma, quote\"\" and\nnewline\"\n" {
		t.Fatalf("escaped table = %q", table.Text)
	}
}

func TestBuilderDoesNotPartiallyAdmitFirstOverBudgetRow(t *testing.T) {
	builder, _ := NewBuilder(codePointCounter{})
	header, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationReports, Columns: []string{"content"},
		Delimiter: '|', MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("Build header: %v", err)
	}
	table, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationReports, Columns: []string{"content"},
		Rows: []Row{{
			Values: []string{"too large"}, CorporaID: "corpus", TextUnitIDs: []string{"unit"},
		}},
		Delimiter: '|', MaxTokens: header.TokenCount,
	})
	if err != nil || len(table.Rows) != 0 || !table.Truncated || table.Text != header.Text {
		t.Fatalf("table/error = %#v / %v", table, err)
	}
}

func TestBuilderReturnsIsolatedRowsWithConfiguredStartingID(t *testing.T) {
	builder, _ := NewBuilder(codePointCounter{})
	row := Row{
		Values: []string{"value"}, CorporaID: "corpus", TextUnitIDs: []string{"unit"},
	}
	table, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationClaims, Columns: []string{"statement"}, Rows: []Row{row},
		Delimiter: ',', MaxTokens: 1000, Prefix: "history\n", StartRecordID: 4,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	row.Values[0] = "changed"
	row.TextUnitIDs[0] = "changed"
	if table.Rows[0].CitationRecord.Reference.RecordID != 4 || table.Rows[0].Values[1] != "value" ||
		table.Rows[0].CitationRecord.TextUnitIDs[0] != "unit" || table.Text[:8] != "history\n" {
		t.Fatalf("isolated table = %#v", table)
	}
}

func TestBuilderRejectsInvalidTableShapeBeforeCounting(t *testing.T) {
	builder, _ := NewBuilder(codePointCounter{})
	_, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationSources, Columns: []string{"text"},
		Rows:      []Row{{Values: nil, CorporaID: "corpus", TextUnitIDs: []string{"unit"}}},
		Delimiter: '|', MaxTokens: 1,
	})
	if err == nil {
		t.Fatal("Build() accepted a row whose value count does not match columns")
	}
}

func TestBuilderRejectsHeaderThatCannotFitHardBudget(t *testing.T) {
	builder, _ := NewBuilder(codePointCounter{})
	_, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationSources, Columns: []string{"text"},
		Delimiter: '|', MaxTokens: 1,
	})
	if !errors.Is(err, ErrBudgetTooSmall) {
		t.Fatalf("Build() error = %v, want ErrBudgetTooSmall", err)
	}
}

func TestBuilderValidatesEveryCandidateBeforeBudgetAdmission(t *testing.T) {
	builder, _ := NewBuilder(codePointCounter{})
	first, err := builder.Build(t.Context(), Request{
		Dataset: querybase.CitationSources, Columns: []string{"text"},
		Rows: []Row{{
			Values: []string{"first"}, CorporaID: "corpus", TextUnitIDs: []string{"unit"},
		}},
		Delimiter: '|', MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("Build first row: %v", err)
	}
	_, err = builder.Build(t.Context(), Request{
		Dataset: querybase.CitationSources, Columns: []string{"text"},
		Rows: []Row{
			{Values: []string{"first"}, CorporaID: "corpus", TextUnitIDs: []string{"unit"}},
			{Values: []string{"never admitted"}, CorporaID: "", TextUnitIDs: nil},
		},
		Delimiter: '|', MaxTokens: first.TokenCount,
	})
	if err == nil {
		t.Fatal("Build() did not validate a candidate after the budget boundary")
	}
}

func TestBuilderRejectsConflictingColumnsAndInvalidDelimiter(t *testing.T) {
	builder, _ := NewBuilder(codePointCounter{})
	for _, request := range []Request{
		{
			Dataset: querybase.CitationSources, Columns: []string{"id"},
			Delimiter: '|', MaxTokens: 100,
		},
		{
			Dataset: querybase.CitationSources, Columns: []string{"text", "text"},
			Delimiter: '|', MaxTokens: 100,
		},
		{
			Dataset: querybase.CitationSources, Columns: []string{"text"},
			Delimiter: utf8.RuneError, MaxTokens: 100,
		},
	} {
		if _, err := builder.Build(t.Context(), request); err == nil {
			t.Fatalf("Build() accepted invalid request %#v", request)
		}
	}
}
