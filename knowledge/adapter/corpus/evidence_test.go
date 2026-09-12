package corpus

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corpusdomain "github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/knowledge/provenance"
)

const evidenceZoneID = "10000000-0000-4000-8000-000000000001"

type evidenceCorpus struct {
	requests []evidenceRequest
	known    map[corpusdomain.CorporaID]map[textunits.TextUnitID]struct{}
	err      error
}

type evidenceRequest struct {
	CorporaID   corpusdomain.CorporaID
	TextUnitIDs []textunits.TextUnitID
}

type evidenceMessages struct {
	known map[string]message.Occurrence
	err   error
}

func (source *evidenceMessages) Read(
	_ context.Context,
	ids []string,
) ([]message.Occurrence, error) {
	if source.err != nil {
		return nil, source.err
	}
	result := make([]message.Occurrence, 0, len(ids))
	for _, id := range ids {
		if occurrence, found := source.known[id]; found {
			result = append(result, occurrence)
		}
	}
	return result, nil
}

func (source *evidenceCorpus) TextUnitLocations(
	_ context.Context,
	corporaID corpusdomain.CorporaID,
	textUnitIDs []textunits.TextUnitID,
) ([]corpusdomain.TextUnitLocation, error) {
	source.requests = append(source.requests, evidenceRequest{
		CorporaID:   corporaID,
		TextUnitIDs: append([]textunits.TextUnitID(nil), textUnitIDs...),
	})
	if source.err != nil {
		return nil, source.err
	}
	var result []corpusdomain.TextUnitLocation
	for _, textUnitID := range textUnitIDs {
		if _, found := source.known[corporaID][textUnitID]; found {
			result = append(result, corpusdomain.TextUnitLocation{
				CorporaID: corporaID,
				TextUnit: textunits.TextUnit{
					TextUnit: textunits.TextUnitBody{
						ID: textUnitID,
					},
				},
			})
		}
	}
	return result, nil
}

func TestEvidenceVerifierChecksExactCorporaAndTextUnitPairs(t *testing.T) {
	source := &evidenceCorpus{
		known: map[corpusdomain.CorporaID]map[textunits.TextUnitID]struct{}{
			"corpora-a": {
				"unit-1": {},
			},
			"corpora-b": {
				"unit-2": {},
			},
		},
	}
	verifier, err := NewEvidenceVerifier(source, &evidenceMessages{})
	if err != nil {
		t.Fatal(err)
	}
	err = verifier.VerifyEvidence(t.Context(), []provenance.Evidence{
		{
			ZoneID:     evidenceZoneID,
			TextUnitID: "unit-1",
			Source:     provenance.CorporaSource{CorporaID: "corpora-a"},
		},
		{
			ZoneID:     evidenceZoneID,
			TextUnitID: "unit-2",
			Source:     provenance.CorporaSource{CorporaID: "corpora-b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []evidenceRequest{
		{
			CorporaID:   "corpora-a",
			TextUnitIDs: []textunits.TextUnitID{"unit-1"},
		},
		{
			CorporaID:   "corpora-b",
			TextUnitIDs: []textunits.TextUnitID{"unit-2"},
		},
	}
	if !reflect.DeepEqual(source.requests, want) {
		t.Fatalf("Corpus requests = %#v, want %#v", source.requests, want)
	}
}

func TestEvidenceVerifierRejectsMissingTextUnitInRequestedCorpora(t *testing.T) {
	source := &evidenceCorpus{
		known: map[corpusdomain.CorporaID]map[textunits.TextUnitID]struct{}{
			"corpora-a": {},
		},
	}
	verifier, err := NewEvidenceVerifier(source, &evidenceMessages{})
	if err != nil {
		t.Fatal(err)
	}
	err = verifier.VerifyEvidence(t.Context(), []provenance.Evidence{
		{
			ZoneID:     evidenceZoneID,
			TextUnitID: "missing",
			Source:     provenance.CorporaSource{CorporaID: "corpora-a"},
		},
	})
	if !errors.Is(err, provenance.ErrInvalidSource) {
		t.Fatalf("VerifyEvidence() error = %v", err)
	}
}

func TestEvidenceVerifierPreservesCorpusFailure(t *testing.T) {
	want := errors.New("Corpus unavailable")
	verifier, err := NewEvidenceVerifier(&evidenceCorpus{
		err: want,
	}, &evidenceMessages{})
	if err != nil {
		t.Fatal(err)
	}
	err = verifier.VerifyEvidence(t.Context(), []provenance.Evidence{
		{
			ZoneID:     evidenceZoneID,
			TextUnitID: "unit-1",
			Source:     provenance.CorporaSource{CorporaID: "corpora-a"},
		},
	})
	if !errors.Is(err, want) {
		t.Fatalf("VerifyEvidence() error = %v", err)
	}
}

func TestEvidenceVerifierChecksExactMessageTextUnit(t *testing.T) {
	value, err := message.New("message-1", "user", "remember this")
	if err != nil {
		t.Fatal(err)
	}
	messages := &evidenceMessages{known: map[string]message.Occurrence{
		value.ID: {Message: value, Position: 3},
	}}
	verifier, err := NewEvidenceVerifier(&evidenceCorpus{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	err = verifier.VerifyEvidence(t.Context(), []provenance.Evidence{{
		ZoneID:     evidenceZoneID,
		TextUnitID: string(value.TextUnit.ID),
		Source:     provenance.MessageSource{MessageID: value.ID},
	}})
	if err != nil {
		t.Fatal(err)
	}

	err = verifier.VerifyEvidence(t.Context(), []provenance.Evidence{{
		ZoneID:     evidenceZoneID,
		TextUnitID: "another-unit",
		Source:     provenance.MessageSource{MessageID: value.ID},
	}})
	if !errors.Is(err, provenance.ErrInvalidSource) {
		t.Fatalf("mismatched Message Evidence error = %v", err)
	}
}

func TestNewEvidenceVerifierRequiresCorpus(t *testing.T) {
	if _, err := NewEvidenceVerifier(nil, &evidenceMessages{}); err == nil {
		t.Fatal("NewEvidenceVerifier(nil) error = nil")
	}
	if _, err := NewEvidenceVerifier(&evidenceCorpus{}, nil); err == nil {
		t.Fatal("NewEvidenceVerifier(corpus, nil) error = nil")
	}
}
