// Package application exposes the unified Query use cases consumed by delivery adapters.
package application

import querybase "github.com/memoria-space/meking/query"

type Method string

const (
	MethodBasic  Method = "basic"
	MethodLocal  Method = "local"
	MethodGlobal Method = "global"
	MethodDRIFT  Method = "drift"
)

type Request struct {
	Method                    Method
	Question                  string
	ResponseType              string
	Conversation              []querybase.ConversationTurn
	CommunityLevel            *int
	DynamicCommunitySelection bool
	IncludeEntityIDs          []string
	ExcludeEntityIDs          []string
}

type Execution struct {
	Method         Method
	EpochID        int64
	ReportSetID    string
	CommunitySetID string
	CorporaID      string
	Response       string
	CitationAudit  querybase.CitationAudit
	Global         *GlobalStatistics
}

type GlobalStatistics struct {
	MapBatches       int
	FailedMapBatches int
	ReduceTokens     int
	ReducePoints     int
	ReduceTruncated  bool
}

type SuggestionRequest struct {
	History []string
	Count   int
}

type SuggestionExecution struct {
	Questions      []string
	EpochID        int64
	ReportSetID    string
	CommunitySetID string
	CorporaID      string
}
