package citation

import (
	"context"
	"errors"
	"reflect"
	"testing"

	querybase "github.com/memoria-space/meking/query"
	querysource "github.com/memoria-space/meking/query/source"
)

type sourceReaderStub struct {
	requests []sourceRequestStub
	sources  map[string][]querysource.TextUnitSource
	err      error
}

type sourceRequestStub struct {
	corporaID   string
	textUnitIDs []string
}

func (s *sourceReaderStub) Read(
	_ context.Context,
	corporaID string,
	textUnitIDs []string,
) ([]querysource.TextUnitSource, error) {
	s.requests = append(s.requests, sourceRequestStub{
		corporaID:   corporaID,
		textUnitIDs: append([]string(nil), textUnitIDs...),
	})
	return append([]querysource.TextUnitSource(nil), s.sources[corporaID]...), s.err
}

func TestAuditResolvesVisibleRecordsAcrossTheirOwnCorpora(t *testing.T) {
	reader := &sourceReaderStub{sources: map[string][]querysource.TextUnitSource{
		"corpus-a": {{
			CorporaID: "corpus-a", TextUnitID: "unit-a", Text: "A",
			DocumentID: "document-a", TextTitle: "a.txt",
		}},
		"corpus-b": {{
			CorporaID: "corpus-b", TextUnitID: "unit-b", Text: "B",
			DocumentID: "document-b", TextTitle: "b.txt",
		}},
	}}
	audit, err := Audit(
		t.Context(),
		"Answer [Data: Reports (7, 8); Entities (1); Reports (9)].",
		[]querybase.CitationRecord{
			{
				Reference: querybase.CitationReference{
					Dataset: querybase.CitationReports, RecordID: 7,
				},
				CorporaID: "corpus-a", TextUnitIDs: []string{"unit-a"},
			},
			{
				Reference: querybase.CitationReference{
					Dataset: querybase.CitationReports, RecordID: 8,
				},
				CorporaID: "corpus-b", TextUnitIDs: []string{"unit-b"},
			},
		},
		reader,
	)
	if err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	if !reflect.DeepEqual(reader.requests, []sourceRequestStub{
		{corporaID: "corpus-a", textUnitIDs: []string{"unit-a"}},
		{corporaID: "corpus-b", textUnitIDs: []string{"unit-b"}},
	}) {
		t.Fatalf("source requests = %#v", reader.requests)
	}
	if audit.Missing || len(audit.Items) != 4 ||
		audit.Items[0].Status != querybase.CitationValid ||
		audit.Items[0].Sources[0].TextTitle != "a.txt" ||
		audit.Items[1].Status != querybase.CitationValid ||
		audit.Items[1].Sources[0].TextTitle != "b.txt" ||
		audit.Items[2].InvalidReason != querybase.CitationOutsideContext ||
		audit.Items[3].InvalidReason != querybase.CitationOutsideContext {
		t.Fatalf("Citation audit = %#v", audit)
	}
}

func TestAuditAnnotatesMissingAndUnresolvedWithoutSourceFailure(t *testing.T) {
	reader := &sourceReaderStub{}
	audit, err := Audit(t.Context(), "No citations", nil, reader)
	if err != nil || !audit.Missing || len(reader.requests) != 0 {
		t.Fatalf("missing audit = %#v, requests = %#v, error = %v", audit, reader.requests, err)
	}

	audit, err = Audit(
		t.Context(),
		"[Data: Reports (0)]",
		[]querybase.CitationRecord{{
			Reference: querybase.CitationReference{
				Dataset: querybase.CitationReports, RecordID: 0,
			},
			CorporaID: "corpora", TextUnitIDs: []string{"unit"},
		}},
		reader,
	)
	if err != nil || len(audit.Items) != 1 ||
		audit.Items[0].InvalidReason != querybase.CitationUnresolvedSource {
		t.Fatalf("unresolved audit = %#v, error = %v", audit, err)
	}
}

func TestAuditRejectsDuplicateScopeAndPropagatesSourceFailure(t *testing.T) {
	reference := querybase.CitationReference{
		Dataset: querybase.CitationReports, RecordID: 0,
	}
	reader := &sourceReaderStub{}
	_, err := Audit(
		t.Context(),
		"[Data: Reports (0)]",
		[]querybase.CitationRecord{
			{Reference: reference, CorporaID: "corpus", TextUnitIDs: []string{"unit"}},
			{Reference: reference, CorporaID: "corpus", TextUnitIDs: []string{"unit"}},
		},
		reader,
	)
	if err == nil {
		t.Fatal("Audit() accepted duplicate record identity")
	}

	want := errors.New("Corpus unavailable")
	reader.err = want
	_, err = Audit(
		t.Context(),
		"[Data: Reports (0)]",
		[]querybase.CitationRecord{{
			Reference: reference, CorporaID: "corpus", TextUnitIDs: []string{"unit"},
		}},
		reader,
	)
	if !errors.Is(err, want) {
		t.Fatalf("Audit() error = %v", err)
	}
}

func TestAuditRejectsDuplicateScopeEvenWhenResponseHasNoMarker(t *testing.T) {
	reference := querybase.CitationReference{
		Dataset: querybase.CitationSources, RecordID: 0,
	}
	_, err := Audit(
		t.Context(),
		"No marker",
		[]querybase.CitationRecord{
			{Reference: reference, CorporaID: "corpus", TextUnitIDs: []string{"unit"}},
			{Reference: reference, CorporaID: "corpus", TextUnitIDs: []string{"unit"}},
		},
		&sourceReaderStub{},
	)
	if err == nil {
		t.Fatal("Audit() accepted duplicate record identity without a model marker")
	}
}
