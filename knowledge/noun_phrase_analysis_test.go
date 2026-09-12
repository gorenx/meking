package knowledge

import (
	"slices"
	"testing"
)

func TestDefaultNounPhraseAnalysisConfigMatchesEnglishFastPolicy(t *testing.T) {
	t.Parallel()

	first := DefaultNounPhraseAnalysisConfig()
	second := DefaultNounPhraseAnalysisConfig()
	wantExcluded := []string{
		"stuff", "thing", "things", "bunch", "bit", "bits", "people",
		"person", "okay", "hey", "hi", "hello", "laughter", "oh",
	}
	if first.ExtractorType != NounPhraseRegexEnglish ||
		first.MaxWordLength != 15 || first.WordDelimiter != " " ||
		!slices.Equal(first.ExcludeNouns, wantExcluded) {
		t.Fatalf("default noun phrase config = %#v", first)
	}
	first.ExcludeNouns[0] = "changed"
	if second.ExcludeNouns[0] != "stuff" {
		t.Fatal("default noun phrase configs share mutable excluded nouns")
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("Validate(default) error = %v", err)
	}
}

func TestNounPhraseAnalysisConfigValidatesResultIdentity(t *testing.T) {
	t.Parallel()
	valid := NounPhraseAnalysisConfig{
		ExtractorType: NounPhraseRegexEnglish,
		ExcludeNouns:  []string{"THING", "PERSON"},
		MaxWordLength: 15,
		WordDelimiter: " ",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, config := range []NounPhraseAnalysisConfig{
		{},
		{ExtractorType: NounPhraseRegexEnglish, MaxWordLength: -1, WordDelimiter: " "},
		{ExtractorType: NounPhraseRegexEnglish, MaxWordLength: 15, WordDelimiter: "\n"},
		{ExtractorType: NounPhraseRegexEnglish, MaxWordLength: 15, WordDelimiter: " ", ExcludeNouns: []string{""}},
	} {
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate(%#v) error = nil", config)
		}
	}
}

func TestValidateNounPhraseResultsEnforcesAlignmentAndRegexIdentity(t *testing.T) {
	t.Parallel()
	config := DefaultNounPhraseAnalysisConfig()
	inputs := []NounPhraseTextUnit{{ID: "one", Text: "Beta"}}
	valid := []NounPhraseResult{{TextUnitID: "one", Phrases: []string{"BETA"}}}
	if err := ValidateNounPhraseResults(inputs, valid, config); err != nil {
		t.Fatalf("ValidateNounPhraseResults() error = %v", err)
	}
	for _, results := range [][]NounPhraseResult{
		nil,
		{{TextUnitID: "other", Phrases: []string{"BETA"}}},
		{{TextUnitID: "one", Phrases: []string{"Beta"}}},
		{{TextUnitID: "one", Phrases: []string{"BETA", "ALPHA"}}},
	} {
		if err := ValidateNounPhraseResults(inputs, results, config); err == nil {
			t.Fatalf("results %#v were accepted", results)
		}
	}
}
