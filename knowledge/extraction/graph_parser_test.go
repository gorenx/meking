package extraction

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseGraphExtractionParsesEntitiesRelationsAndSource(t *testing.T) {
	result := `{
		"entities":[{"name":"Alice","type":"person","aliases":[],"description":"Alice leads"},{"name":"Bob","type":"person","aliases":[],"description":"Bob works"}],
		"relations":[{"source":"Alice","target":"Bob","type":"manages","description":"Alice <|> manages Bob","weight":2.5}]
	}`

	got, err := parseGraphExtraction(result, "tu-1")
	if err != nil {
		t.Fatalf("parseGraphExtraction() error = %v", err)
	}
	want := graphObservations{
		Entities: []entityObservation{
			{Title: "ALICE", Type: "PERSON", Aliases: []string{}, Description: "Alice leads", TextUnitID: "tu-1"},
			{Title: "BOB", Type: "PERSON", Aliases: []string{}, Description: "Bob works", TextUnitID: "tu-1"},
		},
		Relationships: []relationshipObservation{{
			Source: "ALICE", Target: "BOB", Type: "MANAGES", Description: "Alice <|> manages Bob", TextUnitID: "tu-1", Weight: 2.5,
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGraphExtraction() = %#v, want %#v", got, want)
	}
}

func TestParseGraphExtractionCombinesGleaningResults(t *testing.T) {
	result := `{"entities":[{"name":"A","type":"thing","aliases":[],"description":"first"}],"relations":[]}
		{"entities":[{"name":"B","type":"thing","aliases":[],"description":"second"}],"relations":[{"source":"A","target":"B","type":"linked","description":"linked","weight":1}]}`

	got, err := parseGraphExtraction(result, "tu")
	if err != nil {
		t.Fatalf("parseGraphExtraction() error = %v", err)
	}
	if len(got.Entities) != 2 || len(got.Relationships) != 1 {
		t.Fatalf("parseGraphExtraction() = %#v", got)
	}
}

func TestParseGraphExtractionCanonicalizesAliases(t *testing.T) {
	result := `{"entities":[{"name":"Alice","type":"person","aliases":["Ally","alice","A&amp;B","ally","","Beta"],"description":"Alice leads"}],"relations":[]}`

	got, err := parseGraphExtraction(result, "tu-1")
	if err != nil {
		t.Fatalf("parseGraphExtraction() error = %v", err)
	}
	want := []string{"A&B", "ALLY", "BETA"}
	if !reflect.DeepEqual(got.Entities[0].Aliases, want) {
		t.Fatalf("aliases = %#v, want %#v", got.Entities[0].Aliases, want)
	}
}

func TestParseGraphExtractionRejectsMalformedJSONWithCorrectionContext(t *testing.T) {
	result := `{"entities":[],"relations":[{"source":"A"}]`
	_, err := parseGraphExtraction(result, "tu")
	if !errors.Is(err, ErrInvalidGraphExtraction) {
		t.Fatalf("parseGraphExtraction() error = %v, want ErrInvalidGraphExtraction", err)
	}
	var rejection *rejectedResult
	if !errors.As(err, &rejection) || rejection.part != result || !strings.Contains(rejection.reason, "valid graph JSON") {
		t.Fatalf("rejection = %#v", rejection)
	}
}

func TestParseGraphExtractionRejectsInvalidStructureAndFields(t *testing.T) {
	tests := []string{
		`{"entities":[]}`,
		`{"entities":[],"relations":[],"extra":true}`,
		`{"entities":[{"name":"A","type":"thing","aliases":null,"description":"a"}],"relations":[]}`,
		`{"entities":[{"name":"","type":"thing","aliases":[],"description":"a"}],"relations":[]}`,
		`{"entities":[],"relations":[{"source":"A","target":"B","type":"linked","description":"link","weight":null}]}`,
		`{"entities":[],"relations":[{"source":"A","target":"B","type":"linked","description":"link","weight":-1}]}`,
	}
	for _, result := range tests {
		if _, err := parseGraphExtraction(result, "tu"); !errors.Is(err, ErrInvalidGraphExtraction) {
			t.Errorf("parseGraphExtraction(%q) error = %v, want ErrInvalidGraphExtraction", result, err)
		}
	}
}

func TestParseGraphExtractionReturnsNonNilEmptySlices(t *testing.T) {
	got, err := parseGraphExtraction(`{"entities":[],"relations":[]}`, "tu")
	if err != nil {
		t.Fatalf("parseGraphExtraction() error = %v", err)
	}
	if got.Entities == nil || got.Relationships == nil || len(got.Entities) != 0 || len(got.Relationships) != 0 {
		t.Fatalf("parseGraphExtraction() = %#v, want non-nil empty slices", got)
	}
}

func TestParseGraphExtractionUsesFullUnicodeUppercase(t *testing.T) {
	got, err := parseGraphExtraction(`{"entities":[{"name":"Straße","type":"größe","aliases":[],"description":"description"}],"relations":[]}`, "tu")
	if err != nil {
		t.Fatalf("parseGraphExtraction() error = %v", err)
	}
	if len(got.Entities) != 1 || got.Entities[0].Title != "STRASSE" || got.Entities[0].Type != "GRÖSSE" {
		t.Fatalf("entity = %#v, want full Unicode uppercase", got.Entities)
	}
}
