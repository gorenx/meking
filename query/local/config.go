package local

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultTopKEntities bounds semantic Entity matches before explicit request
	// filters and exact Knowledge reads.
	DefaultTopKEntities = 10
	// DefaultTopKRelationships bounds external graph expansion per selected
	// Entity after all relationships between selected Entities are retained.
	DefaultTopKRelationships = 10
	// DefaultMaxContextTokens is the hard token budget for conversation and all
	// model-visible evidence tables in one Local request.
	DefaultMaxContextTokens = 12000
	// DefaultCommunityProportion reserves this fraction of the post-conversation
	// budget for Community Reports.
	DefaultCommunityProportion = 0.15
	// DefaultTextUnitProportion reserves this fraction of the post-conversation
	// budget for source TextUnits.
	DefaultTextUnitProportion = 0.5
	// DefaultConversationTurns limits the number of most recent user-led
	// exchanges copied into the Local prompt.
	DefaultConversationTurns = 5
	// DefaultResponseType is inserted into the Local answer prompt when a request
	// does not provide an override.
	DefaultResponseType = "multiple paragraphs"
)

// Config owns bounded Local retrieval and context allocation. It solves the
// problem of otherwise unbounded graph expansion while keeping all choices
// immutable for a reusable Searcher; request input cannot change these limits.
type Config struct {
	// TopKEntities is the positive number of Entity matches retained after the
	// vector reader returns an oversampled candidate list.
	TopKEntities int
	// TopKRelationships is the positive maximum external Relation count per
	// selected Entity. Relations whose two endpoints are selected are retained
	// before this bound is applied.
	TopKRelationships int
	// MaxContextTokens is the positive hard token budget for the fully assembled
	// conversation and evidence text.
	MaxContextTokens int
	// CommunityProportion is in [0,1] and reserves that fraction of the budget
	// remaining after conversation for Report rows.
	CommunityProportion float64
	// TextUnitProportion is in [0,1]; together with CommunityProportion it must
	// not exceed one. The remainder is used by Entity, Relationship, and Claim rows.
	TextUnitProportion float64
	// ConversationTurns is the positive maximum number of recent user-led
	// exchanges rendered before evidence selection.
	ConversationTurns int
	// ConversationUserTurnsOnly omits assistant and system responses from the
	// rendered history while still grouping history by user-led exchange.
	ConversationUserTurnsOnly bool
	// Delimiter is one non-quote line rune used by every Local evidence table.
	Delimiter rune
	// ResponseType is the required default answer length and presentation text.
	ResponseType string
}

// DefaultConfig returns the built-in bounded Local policy.
func DefaultConfig() Config {
	return Config{
		TopKEntities: DefaultTopKEntities, TopKRelationships: DefaultTopKRelationships,
		MaxContextTokens:    DefaultMaxContextTokens,
		CommunityProportion: DefaultCommunityProportion,
		TextUnitProportion:  DefaultTextUnitProportion,
		ConversationTurns:   DefaultConversationTurns, ConversationUserTurnsOnly: true,
		Delimiter: '|', ResponseType: DefaultResponseType,
	}
}

// Validate rejects policies that cannot produce a bounded Local request.
func (c Config) Validate() error {
	switch {
	case c.TopKEntities <= 0:
		return errors.New("Local Search Entity limit must be positive")
	case c.TopKRelationships <= 0:
		return errors.New("Local Search Relationship limit must be positive")
	case c.MaxContextTokens <= 0:
		return errors.New("Local Search context token budget must be positive")
	case c.CommunityProportion < 0 || c.CommunityProportion > 1:
		return errors.New("Local Search Community proportion must be between zero and one")
	case c.TextUnitProportion < 0 || c.TextUnitProportion > 1:
		return errors.New("Local Search TextUnit proportion must be between zero and one")
	case c.CommunityProportion+c.TextUnitProportion > 1:
		return errors.New("Local Search Community and TextUnit proportions must not exceed one")
	case c.ConversationTurns <= 0:
		return errors.New("Local Search conversation turn limit must be positive")
	case c.Delimiter == 0 || c.Delimiter == '"' || c.Delimiter == '\r' ||
		c.Delimiter == '\n' || c.Delimiter == utf8.RuneError:
		return errors.New("Local Search delimiter must be one non-quote line rune")
	case strings.TrimSpace(c.ResponseType) == "" || c.ResponseType != strings.TrimSpace(c.ResponseType):
		return errors.New("Local Search default response type is required without surrounding whitespace")
	default:
		return nil
	}
}
