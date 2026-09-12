package citation

import (
	"reflect"
	"testing"

	querybase "github.com/memoria-space/meking/query"
)

func TestParseReferencesPreservesModelOrderAndSeparatesMissingFromInvalid(t *testing.T) {
	references, found, err := ParseReferences(
		"[Data: Reports (2, 0, 2); Sources (4)]",
	)
	want := []querybase.CitationReference{
		{Dataset: querybase.CitationReports, RecordID: 2},
		{Dataset: querybase.CitationReports, RecordID: 0},
		{Dataset: querybase.CitationReports, RecordID: 2},
		{Dataset: querybase.CitationSources, RecordID: 4},
	}
	if err != nil || !found || !reflect.DeepEqual(references, want) {
		t.Fatalf("ParseReferences() = %#v, %t, %v", references, found, err)
	}

	references, found, err = ParseReferences("no marker")
	if err != nil || found || len(references) != 0 {
		t.Fatalf("ParseReferences(missing) = %#v, %t, %v", references, found, err)
	}

	references, found, err = ParseReferences("[Data: Reports (-1)]")
	if err == nil || !found || references != nil {
		t.Fatalf("ParseReferences(invalid) = %#v, %t, %v", references, found, err)
	}
}

func TestParseSupportsPromptDatasetsSeparatorsAndMore(t *testing.T) {
	candidates, found := parse(
		"[Data: Sources (0); Reports (1, 2, +more), Entities (3); Relationships (4), Claims (5)]",
	)
	if !found || len(candidates) != 6 {
		t.Fatalf("parse result = %#v, found = %t", candidates, found)
	}
	want := []querybase.CitationDataset{
		querybase.CitationSources,
		querybase.CitationReports,
		querybase.CitationReports,
		querybase.CitationEntities,
		querybase.CitationRelationships,
		querybase.CitationClaims,
	}
	for index, candidate := range candidates {
		if !candidate.parsed || candidate.reference.Dataset != want[index] {
			t.Fatalf("candidate %d = %#v", index, candidate)
		}
	}
}

func TestParseReturnsStableInvalidReasons(t *testing.T) {
	tests := []struct {
		response string
		reason   querybase.CitationInvalidReason
	}{
		{"[Data: Reports (0)", querybase.CitationMalformed},
		{"[Data: Unknown (0)]", querybase.CitationUnsupportedDataset},
		{"[Data: Reports (-1)]", querybase.CitationInvalidRecordID},
		{"[Data: Reports (0, 1, 2, 3, 4, 5)]", querybase.CitationTooManyRecords},
	}
	for _, test := range tests {
		candidates, found := parse(test.response)
		if !found || len(candidates) != 1 || candidates[0].invalidReason != test.reason {
			t.Errorf("parse(%q) = %#v, found = %t", test.response, candidates, found)
		}
	}
}

func TestRewriteReferencesUsesOnlyAcceptedMappings(t *testing.T) {
	text := "first [Data: Entities (0, 1); Sources (0)] second [Data: Unknown (4)]"
	got := RewriteReferences(text, map[querybase.CitationReference]querybase.CitationReference{
		{Dataset: querybase.CitationEntities, RecordID: 0}: {
			Dataset: querybase.CitationEntities, RecordID: 3,
		},
		{Dataset: querybase.CitationSources, RecordID: 0}: {
			Dataset: querybase.CitationSources, RecordID: 2,
		},
	})
	want := "first [Data: Entities (3); Sources (2)] second "
	if got != want {
		t.Fatalf("RewriteReferences() = %q, want %q", got, want)
	}
}

func TestRewriteReferencesChunksCanonicalGroups(t *testing.T) {
	text := "[Data: Sources (0, 1, 2, 3, 4); Sources (5)]"
	replacements := make(map[querybase.CitationReference]querybase.CitationReference)
	for index := range 6 {
		replacements[querybase.CitationReference{
			Dataset: querybase.CitationSources, RecordID: index,
		}] = querybase.CitationReference{
			Dataset: querybase.CitationSources, RecordID: index + 10,
		}
	}
	got := RewriteReferences(text, replacements)
	want := "[Data: Sources (10, 11, 12, 13, 14); Sources (15)]"
	if got != want {
		t.Fatalf("RewriteReferences() = %q, want %q", got, want)
	}
}

func TestRewriteReferencesRemovesUnclosedCitationTail(t *testing.T) {
	if got := RewriteReferences("answer [Data: Sources (0)", nil); got != "answer " {
		t.Fatalf("RewriteReferences() = %q", got)
	}
}
