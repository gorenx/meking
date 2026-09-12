package global

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

const (
	// DefaultContextTokens limits the report table supplied to one Map call.
	DefaultContextTokens = 12000
	// DefaultCommunityLevel includes hierarchy levels zero through two.
	DefaultCommunityLevel = 2
	// DefaultRandomSeed keeps report batching stable across queries.
	DefaultRandomSeed uint64 = 86
	// DefaultHistoryTurns limits optional conversation context.
	DefaultHistoryTurns = 5
)

// ReportCandidate pairs one published immutable Report with its exact Community
// position. Selection implementations read this value but do not mutate it.
type ReportCandidate struct {
	// Report contains the model-generated content, rank, and exact source IDs.
	Report queryreport.PublishedReport
	// Community contains the stable membership ID, hierarchy level, and parent.
	Community queryreport.PublishedCommunity
}

// SelectionRequest contains the level-filtered Reports that a
// question-dependent selector may inspect. Rank filtering happens after
// selection so a low-ranked parent cannot hide otherwise relevant descendants.
type SelectionRequest struct {
	// Question is rated against Reports.
	Question string
	// Reports preserve deterministic CommunitySet order.
	Reports []ReportCandidate
}

// CommunitySelector returns selected Community IDs in deterministic handoff
// order. Every returned ID must identify one ReportCandidate in the request.
type CommunitySelector interface {
	SelectCommunities(ctx context.Context, request SelectionRequest) ([]string, error)
}

// ContextConfig controls Report selection, exact-version occurrence weights,
// prompt ordering, and per-Map token allocation.
type ContextConfig struct {
	// MaxContextTokens is the positive report-table budget for each Map call.
	MaxContextTokens int
	// CommunityLevel includes levels zero through this value; nil disables it.
	CommunityLevel *int
	// UseCommunitySummary chooses Summary instead of FullContent in Report rows.
	UseCommunitySummary bool
	// ShuffleData applies the deterministic RandomSeed before token batching.
	ShuffleData bool
	// IncludeCommunityRank renders Rank and uses it as a secondary batch sort key.
	IncludeCommunityRank bool
	// MinimumCommunityRank excludes lower-ranked Reports after dynamic selection,
	// preserving the hierarchy path used for model relevance traversal.
	MinimumCommunityRank float64
	// IncludeCommunityWeight renders unique Entity TextUnit occurrence weight.
	IncludeCommunityWeight bool
	// NormalizeCommunityWeight divides selected weights by the largest weight.
	NormalizeCommunityWeight bool
	// ConversationHistoryTurns is the positive maximum number of recent user turns.
	ConversationHistoryTurns int
	// ConversationUserTurnsOnly omits assistant rows from rendered history.
	ConversationUserTurnsOnly bool
	// ColumnDelimiter is one non-quote, non-newline table separator rune.
	ColumnDelimiter string
	// ContextName is the non-empty heading for model-visible Report rows.
	ContextName string
	// RandomSeed initializes deterministic Report shuffling.
	RandomSeed uint64
	// DynamicSelection enables the explicitly configured CommunitySelector.
	DynamicSelection bool
}

// DefaultContextConfig returns the built-in static Report batching policy.
func DefaultContextConfig() ContextConfig {
	level := DefaultCommunityLevel
	return ContextConfig{
		MaxContextTokens:          DefaultContextTokens,
		CommunityLevel:            &level,
		ShuffleData:               true,
		IncludeCommunityRank:      true,
		IncludeCommunityWeight:    true,
		NormalizeCommunityWeight:  true,
		ConversationHistoryTurns:  DefaultHistoryTurns,
		ConversationUserTurnsOnly: true,
		ColumnDelimiter:           "|",
		ContextName:               "Reports",
		RandomSeed:                DefaultRandomSeed,
	}
}

// Validate rejects policies that cannot produce bounded deterministic context.
func (c ContextConfig) Validate() error {
	if c.MaxContextTokens <= 0 {
		return errors.New("global context token budget must be positive")
	}
	if c.CommunityLevel != nil && *c.CommunityLevel < 0 {
		return errors.New("global Community level must be non-negative")
	}
	if c.MinimumCommunityRank < 0 || math.IsNaN(c.MinimumCommunityRank) ||
		math.IsInf(c.MinimumCommunityRank, 0) {
		return errors.New("global minimum Community rank must be finite and non-negative")
	}
	if c.ConversationHistoryTurns <= 0 {
		return errors.New("global conversation history limit must be positive")
	}
	if len([]rune(c.ColumnDelimiter)) != 1 || strings.ContainsAny(c.ColumnDelimiter, "\r\n\"") {
		return errors.New("global context delimiter must be one non-quote line character")
	}
	if strings.TrimSpace(c.ContextName) == "" || strings.ContainsAny(c.ContextName, "\r\n") {
		return errors.New("global context name is required on one line")
	}
	return nil
}

// ContextRequest supplies one already fixed ReportEvidence value and the
// request-scoped choices used to construct Map evidence.
type ContextRequest struct {
	// Evidence is fixed once by the Global Search session before model work starts.
	Evidence ReportEvidence
	// Question is forwarded only to optional dynamic Report selection.
	Question string
	// Conversation is copied and rendered as a prefix for every Map chunk.
	Conversation []querybase.ConversationTurn
	// Config defines Report filtering, ordering, and token allocation.
	Config ContextConfig
}

// ReportReference is the request-local mapping from a model-visible integer to
// one immutable Report and Community. RecordID is allocated after selection and
// is never persisted or reused as Report identity.
type ReportReference struct {
	// RecordID is the zero-based integer rendered in this request's Reports table.
	RecordID int
	// ReportID is the immutable Community Report UUID.
	ReportID string
	// CommunityID is the stable membership identity summarized by ReportID.
	CommunityID string
	// TextUnitIDs is the immutable evidence set copied from PublishedReport.Sources
	// when this request's Reports table is built. Community Report is the fact
	// source; values remain strictly sorted and duplicate-free for the request
	// lifetime and let Citation resolve evidence without reopening Report storage.
	TextUnitIDs []string
}

// ContextChunk is one independently evaluated Map input after Report ordering
// and token allocation.
type ContextChunk struct {
	// Index is the stable input position used to restore concurrent Map results.
	Index int
	// Text is the exact context_data inserted into the Map system prompt.
	Text string
	// TokenCount records the rendered chunk size for diagnostics.
	TokenCount int
	// Columns and Rows expose the model-visible table without reparsing Text.
	Columns []string
	Rows    []querybase.ContextRow
	// ReportIDs are request-local RecordIDs available for citations in this chunk.
	ReportIDs []int
}

// Context is the fixed evidence audit trail shared by Map, Reduce, and Citation.
type Context struct {
	// EpochID is the positive unified publication fixed before any Report,
	// Knowledge, Corpus, or model work for this request.
	EpochID int64
	// ReportSetID is the immutable publication selected by EpochID.
	ReportSetID string
	// CommunitySetID is the hierarchy summarized by ReportSetID.
	CommunitySetID string
	// CorporaID is the evidence collection used by every selected Report.
	CorporaID string
	// Chunks are evaluated independently in deterministic input order.
	Chunks []ContextChunk
	// Reports maps every accepted request-local RecordID to immutable identities.
	Reports []ReportReference
	// Sections retain model-visible records for Citation auditing.
	Sections []querybase.ContextSection
}

func validateContextRequest(request ContextRequest) error {
	if err := request.Config.Validate(); err != nil {
		return err
	}
	for index, turn := range request.Conversation {
		switch turn.Role {
		case querybase.RoleSystem, querybase.RoleUser, querybase.RoleAssistant:
		default:
			return fmt.Errorf("global conversation turn %d has unsupported role %q", index, turn.Role)
		}
	}
	return nil
}
