package knowledge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maximumNounPhraseWordLength    = 1_000_000
	maximumNounPhraseDelimiterSize = 32
	maximumExcludedNouns           = 4096
	maximumExcludedNounSize        = 256
	maximumNounPhraseSize          = 4096
	defaultNounPhraseWordLength    = 15
)

var defaultEnglishExcludedNouns = [...]string{
	"stuff", "thing", "things", "bunch", "bit", "bits", "people",
	"person", "okay", "hey", "hi", "hello", "laughter", "oh",
}

// NounPhraseExtractor identifies a reproducible phrase-membership policy
// independently of the Sidecar deployment.
type NounPhraseExtractor string

const (
	// NounPhraseRegexEnglish selects the English proper-noun and compound policy.
	NounPhraseRegexEnglish NounPhraseExtractor = "regex_english"
)

// NounPhraseAnalysisConfig holds only phrase-membership decisions. Process,
// cache, transport, and runtime installation policy belong outside knowledge.
type NounPhraseAnalysisConfig struct {
	ExtractorType NounPhraseExtractor
	ExcludeNouns  []string
	MaxWordLength int
	WordDelimiter string
}

// DefaultNounPhraseAnalysisConfig returns the English phrase-membership policy
// used by the English analyzer. Each call owns its excluded-noun slice so one
// consumer cannot mutate another consumer's policy.
func DefaultNounPhraseAnalysisConfig() NounPhraseAnalysisConfig {
	return NounPhraseAnalysisConfig{
		ExtractorType: NounPhraseRegexEnglish,
		ExcludeNouns:  append([]string(nil), defaultEnglishExcludedNouns[:]...),
		MaxWordLength: defaultNounPhraseWordLength,
		WordDelimiter: " ",
	}
}

// Validate rejects ambiguous or unbounded phrase policies before analysis or
// graph construction begins.
func (c NounPhraseAnalysisConfig) Validate() error {
	if c.ExtractorType != NounPhraseRegexEnglish {
		return fmt.Errorf("unsupported noun phrase extractor %q", c.ExtractorType)
	}
	if c.MaxWordLength <= 0 || c.MaxWordLength > maximumNounPhraseWordLength {
		return errors.New("noun phrase maximum word length is invalid")
	}
	if !validNounPhraseConfigurationValue(c.WordDelimiter, maximumNounPhraseDelimiterSize) {
		return errors.New("noun phrase word delimiter is invalid")
	}
	if len(c.ExcludeNouns) > maximumExcludedNouns {
		return errors.New("noun phrase excluded noun list is too large")
	}
	for _, noun := range c.ExcludeNouns {
		if !validNounPhraseConfigurationValue(noun, maximumExcludedNounSize) {
			return errors.New("noun phrase excluded noun is invalid")
		}
	}
	return nil
}

// NounPhraseTextUnit is the minimum source evidence required by analysis. IDs
// may repeat because public TextUnit identity is content-based; ordering still
// distinguishes input rows.
type NounPhraseTextUnit struct {
	ID   string
	Text string
}

// NounPhraseResult preserves input position and source identity while carrying
// only stable phrase membership into graph construction.
type NounPhraseResult struct {
	TextUnitID string
	Phrases    []string
}

// NounPhraseAnalyzer is the knowledge-owned port for an external text-analysis
// capability. Implementations cannot return graph entities or relationships.
type NounPhraseAnalyzer interface {
	AnalyzeNounPhrases(
		ctx context.Context,
		textUnits []NounPhraseTextUnit,
		config NounPhraseAnalysisConfig,
	) ([]NounPhraseResult, error)
}

// ValidateNounPhraseResults proves that an analyzer response is positionally
// aligned and obeys the selected extractor's canonical identity policy before
// phrases are allowed to become graph nodes.
func ValidateNounPhraseResults(
	inputs []NounPhraseTextUnit,
	results []NounPhraseResult,
	config NounPhraseAnalysisConfig,
) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if len(results) != len(inputs) {
		return errors.New("noun phrase result count does not match input")
	}
	for index, result := range results {
		if result.TextUnitID != inputs[index].ID {
			return fmt.Errorf("noun phrase result %d is not aligned to its input", index)
		}
		if err := validateNounPhraseResult(index, result); err != nil {
			return err
		}
		if config.ExtractorType == NounPhraseRegexEnglish {
			for phraseIndex, phrase := range result.Phrases {
				if phrase != strings.ToUpper(phrase) {
					return fmt.Errorf(
						"noun phrase result %d phrase %d is not canonical uppercase",
						index, phraseIndex,
					)
				}
			}
		}
	}
	return nil
}

func validateNounPhraseResult(index int, result NounPhraseResult) error {
	if result.TextUnitID == "" {
		return fmt.Errorf("noun phrase result %d has empty text unit id", index)
	}
	if !slices.IsSorted(result.Phrases) {
		return fmt.Errorf("noun phrase result %d is not sorted", index)
	}
	for phraseIndex, phrase := range result.Phrases {
		if !validNounPhraseConfigurationValue(phrase, maximumNounPhraseSize) {
			return fmt.Errorf("noun phrase result %d has invalid phrase", index)
		}
		if phraseIndex > 0 && phrase == result.Phrases[phraseIndex-1] {
			return fmt.Errorf("noun phrase result %d has duplicate phrase %q", index, phrase)
		}
	}
	return nil
}

func validNounPhraseConfigurationValue(value string, maximumRunes int) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximumRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
