package httpservice

import (
	"time"

	querybase "github.com/memoria-space/meking/query"
	queryapplication "github.com/memoria-space/meking/query/application"
	queryreport "github.com/memoria-space/meking/query/report"
)

const httpContractVersion = 20

type httpRuntimeResponse struct {
	ContractVersion    int      `json:"contract_version"`
	ApplicationVersion string   `json:"application_version"`
	ZoneID             string   `json:"zone_id"`
	EpochID            int64    `json:"epoch_id,omitempty"`
	ReportSetID        string   `json:"report_set_id,omitempty"`
	Ready              bool     `json:"ready"`
	Capabilities       []string `json:"capabilities"`
}

type httpDocumentReceipt struct {
	ZoneID        string `json:"zone_id"`
	DocumentID    string `json:"document_id"`
	ContentDigest string `json:"content_digest"`
	Status        string `json:"status"`
}

type httpDocumentCatalogEntry struct {
	DocumentID    string `json:"document_id"`
	Name          string `json:"name"`
	MediaType     string `json:"media_type"`
	Size          int64  `json:"size"`
	ContentDigest string `json:"content_digest"`
	Selected      bool   `json:"selected"`
}

type httpDocumentCatalogPage struct {
	ZoneID     string                     `json:"zone_id"`
	Offset     int                        `json:"offset"`
	NextOffset *int                       `json:"next_offset,omitempty"`
	HasMore    bool                       `json:"has_more"`
	Documents  []httpDocumentCatalogEntry `json:"documents"`
}

type httpJournalEventPage struct {
	ZoneID     string             `json:"zone_id"`
	Offset     uint64             `json:"offset,string"`
	NextOffset uint64             `json:"next_offset,string"`
	HasMore    bool               `json:"has_more"`
	Events     []httpJournalEvent `json:"events"`
}

type httpJournalEvent struct {
	Sequence       uint64    `json:"sequence,string"`
	EventID        string    `json:"event_id"`
	ZoneID         string    `json:"zone_id"`
	Type           string    `json:"type"`
	SchemaVersion  uint32    `json:"schema_version"`
	StreamID       string    `json:"stream_id"`
	StreamSequence uint64    `json:"stream_sequence,string"`
	OccurredAt     time.Time `json:"occurred_at"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
	CausationID    string    `json:"causation_id,omitempty"`
}

type httpJournalStreamPage struct {
	ZoneID            string             `json:"zone_id"`
	StreamID          string             `json:"stream_id"`
	AfterSequence     uint64             `json:"after_sequence,string"`
	NextAfterSequence uint64             `json:"next_after_sequence,string"`
	HasMore           bool               `json:"has_more"`
	Events            []httpJournalEvent `json:"events"`
}

type httpErrorEnvelope struct {
	Error httpError `json:"error"`
}

type httpError struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	Retryable     bool   `json:"retryable,omitempty"`
	StatusCode    int    `json:"provider_status_code,omitempty"`
	PartialOutput bool   `json:"partial_output,omitempty"`
}

type httpConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type httpQueryRequest struct {
	Method                    string                 `json:"method"`
	Question                  string                 `json:"question"`
	ResponseType              string                 `json:"response_type,omitempty"`
	CommunityLevel            *int                   `json:"community_level,omitempty"`
	DynamicCommunitySelection bool                   `json:"dynamic_community_selection,omitempty"`
	Conversation              []httpConversationTurn `json:"conversation,omitempty"`
	// IncludeEntityIDs and ExcludeEntityIDs are Local-only stable Knowledge Entity IDs.
	IncludeEntityIDs []string `json:"include_entity_ids,omitempty"`
	ExcludeEntityIDs []string `json:"exclude_entity_ids,omitempty"`
}

type httpSuggestionRequest struct {
	// History is ordered oldest to newest; the final item is the current Local question.
	History []string `json:"history"`
	// Count is the positive maximum number of normalized candidates to return.
	Count int `json:"count"`
}

type httpSuggestionResponse struct {
	Questions []string `json:"questions"`
	// EpochID identifies the unified publication fixed by the reused Local context.
	EpochID int64 `json:"epoch_id"`
	// ReportSetID and CommunitySetID identify the fixed Local report publication.
	ReportSetID    string `json:"report_set_id"`
	CommunitySetID string `json:"community_set_id"`
	// CorporaID identifies the source collection used to build Local context.
	CorporaID string `json:"corpora_id"`
}

type httpQueryResponse struct {
	Method string `json:"method"`
	// EpochID is the unified publication fixed by the selected Query method.
	EpochID int64 `json:"epoch_id,omitempty"`
	// ReportSetID and CommunitySetID are set when the method consumes Reports.
	ReportSetID    string `json:"report_set_id,omitempty"`
	CommunitySetID string `json:"community_set_id,omitempty"`
	// CorporaID identifies the immutable TextUnit evidence collection.
	CorporaID     string             `json:"corpora_id,omitempty"`
	Response      string             `json:"response"`
	CitationAudit *httpCitationAudit `json:"citation_audit,omitempty"`
}

type httpKnowledgePublication struct {
	EpochID     int64  `json:"epoch_id"`
	CorporaID   string `json:"corpora_id"`
	ReportSetID string `json:"report_set_id"`
}

type httpKnowledgeEntity struct {
	ID            string   `json:"id"`
	Version       uint64   `json:"version"`
	Title         string   `json:"title"`
	Type          string   `json:"type"`
	Aliases       []string `json:"aliases"`
	Description   string   `json:"description"`
	Degree        int      `json:"degree"`
	EvidenceCount int      `json:"evidence_count"`
}

type httpKnowledgeEntityPage struct {
	httpKnowledgePublication
	Items     []httpKnowledgeEntity `json:"items"`
	NextAfter string                `json:"next_after"`
	HasMore   bool                  `json:"has_more"`
}

type httpKnowledgeRelation struct {
	ID             string  `json:"id"`
	Version        uint64  `json:"version"`
	SourceEntityID string  `json:"source_entity_id"`
	TargetEntityID string  `json:"target_entity_id"`
	Description    string  `json:"description"`
	Weight         float64 `json:"weight"`
	CombinedDegree int     `json:"combined_degree"`
	EvidenceCount  int     `json:"evidence_count"`
}

type httpKnowledgeRelationPage struct {
	httpKnowledgePublication
	Items     []httpKnowledgeRelation `json:"items"`
	NextAfter string                  `json:"next_after"`
	HasMore   bool                    `json:"has_more"`
}

type httpKnowledgeClaimEvidence struct {
	TextUnitID  string `json:"text_unit_id"`
	SubjectText string `json:"subject_text"`
	ObjectText  string `json:"object_text"`
	Status      string `json:"status"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Description string `json:"description"`
	SourceText  string `json:"source_text"`
}

type httpKnowledgeClaim struct {
	ID        string                       `json:"id"`
	Version   uint64                       `json:"version"`
	SubjectID string                       `json:"subject_id"`
	Type      string                       `json:"type"`
	Evidence  []httpKnowledgeClaimEvidence `json:"evidence"`
}

type httpKnowledgeClaimPage struct {
	httpKnowledgePublication
	Items     []httpKnowledgeClaim `json:"items"`
	NextAfter string               `json:"next_after"`
	HasMore   bool                 `json:"has_more"`
}

type httpTextUnit struct {
	CorporaID        string `json:"corpora_id"`
	TextUnitID       string `json:"text_unit_id"`
	Text             string `json:"text"`
	TextID           string `json:"text_id"`
	DocumentID       string `json:"document_id"`
	DocumentLocation string `json:"document_location"`
	TextTitle        string `json:"text_title"`
}

type httpKnowledgeEntityDetail struct {
	httpKnowledgePublication
	Entity    httpKnowledgeEntity     `json:"entity"`
	Relations []httpKnowledgeRelation `json:"relations"`
	Neighbors []httpKnowledgeEntity   `json:"neighbors"`
	Claims    []httpKnowledgeClaim    `json:"claims"`
	TextUnits []httpTextUnit          `json:"text_units"`
}

type httpKnowledgeRelationDetail struct {
	httpKnowledgePublication
	Relation  httpKnowledgeRelation `json:"relation"`
	Endpoints []httpKnowledgeEntity `json:"endpoints"`
	Claims    []httpKnowledgeClaim  `json:"claims"`
	TextUnits []httpTextUnit        `json:"text_units"`
}

type httpGraphEntity struct {
	ID            string   `json:"id"`
	Version       uint64   `json:"version"`
	Title         string   `json:"title"`
	Type          string   `json:"type"`
	Aliases       []string `json:"aliases"`
	Description   string   `json:"description"`
	Degree        int      `json:"degree"`
	TextUnitCount int      `json:"text_unit_count"`
}

type httpGraphRelation struct {
	ID             string  `json:"id"`
	Version        uint64  `json:"version"`
	SourceEntityID string  `json:"source_entity_id"`
	TargetEntityID string  `json:"target_entity_id"`
	Type           string  `json:"type"`
	Description    string  `json:"description"`
	Weight         float64 `json:"weight"`
	CombinedDegree int     `json:"combined_degree"`
	TextUnitCount  int     `json:"text_unit_count"`
}

type httpEntityGraph struct {
	Entities         []httpGraphEntity   `json:"entities"`
	Relations        []httpGraphRelation `json:"relations"`
	MatchedEntities  int                 `json:"matched_entities"`
	MatchedRelations int                 `json:"matched_relations"`
	Truncated        bool                `json:"truncated"`
}

type httpEntityNeighborhood struct {
	Center           httpGraphEntity     `json:"center"`
	Entities         []httpGraphEntity   `json:"entities"`
	Relations        []httpGraphRelation `json:"relations"`
	MatchedRelations int                 `json:"matched_relations"`
	Truncated        bool                `json:"truncated"`
}

type httpGraphCommunity struct {
	ID          string  `json:"id"`
	Number      int     `json:"number"`
	Level       int     `json:"level"`
	ParentID    *string `json:"parent_id,omitempty"`
	ChildCount  int     `json:"child_count"`
	EntityCount int     `json:"entity_count"`
}

type httpCommunityGraph struct {
	StructureID    string               `json:"structure_id"`
	CommunitySetID string               `json:"community_set_id"`
	CorporaID      string               `json:"corpora_id"`
	Page           int                  `json:"page"`
	PageSize       int                  `json:"page_size"`
	Total          int                  `json:"total"`
	Communities    []httpGraphCommunity `json:"communities"`
}

type httpCitationAudit struct {
	Missing bool           `json:"missing"`
	Items   []httpCitation `json:"items"`
}

type httpCitation struct {
	Raw           string               `json:"raw"`
	Dataset       string               `json:"dataset,omitempty"`
	RecordID      *int                 `json:"record_id,omitempty"`
	Parsed        bool                 `json:"parsed"`
	Status        string               `json:"status"`
	InvalidReason string               `json:"invalid_reason,omitempty"`
	Sources       []httpCitationSource `json:"sources"`
}

type httpCitationSource struct {
	// CorporaID identifies the immutable source collection opened by Citation.
	CorporaID        string `json:"corpora_id"`
	TextUnitID       string `json:"text_unit_id"`
	Text             string `json:"text"`
	DocumentID       string `json:"document_id"`
	DocumentLocation string `json:"document_location"`
	TextTitle        string `json:"document_title"`
}

// httpReportPage is the JSON navigation result for one ReportSet-fixed browse
// request. Its publication IDs let the client identify the exact result set.
type httpReportPage struct {
	// EpochID is the unified publication fixed for this page request.
	EpochID int64 `json:"epoch_id"`
	// ReportSetID is the immutable report selection named by EpochID.
	ReportSetID string `json:"report_set_id"`
	// CommunitySetID identifies the hierarchy summarized by ReportSetID.
	CommunitySetID string `json:"community_set_id"`
	// CorporaID identifies the evidence collection used by the selected Reports.
	CorporaID string `json:"corpora_id"`
	// Page is the echoed one-based page requested by the client.
	Page int `json:"page"`
	// PageSize is the echoed positive per-page bound.
	PageSize int `json:"page_size"`
	// Total counts Reports in the immutable ReportSet selected by EpochID.
	Total int `json:"total"`
	// Reports is the deterministic Community-order page and is always a JSON array.
	Reports []httpReportSummary `json:"reports"`
}

// httpReportSummary is the ReportSet navigation projection used by the report
// list; it contains no copied source content.
type httpReportSummary struct {
	// ID is the immutable Report UUID accepted by the detail endpoint.
	ID string `json:"id"`
	// CommunityID is the immutable Entity-membership identity summarized by ID.
	CommunityID string `json:"community_id"`
	// CommunityNumber is the CommunitySet-local non-negative display ordinal.
	CommunityNumber int `json:"community_number"`
	// Level is the hierarchy depth, where roots use zero.
	Level int `json:"level"`
	// Title is the persisted model-generated Report title.
	Title string `json:"title"`
	// Summary is the persisted model-generated executive summary.
	Summary string `json:"summary"`
	// Rank is the finite model-generated impact rating.
	Rank float64 `json:"rank"`
	// Period is the business period fixed when the Report was generated.
	Period string `json:"period"`
	// Size is the number of Entity IDs in the exact Community membership.
	Size int `json:"size"`
}

// httpReportDetail is the JSON representation of one visible immutable Report,
// its hierarchy position, and the exact source identities used at generation.
type httpReportDetail struct {
	// EpochID is the unified publication fixed for this detail request.
	EpochID int64 `json:"epoch_id"`
	// ReportSetID is the immutable publication fixed for this detail request.
	ReportSetID string `json:"report_set_id"`
	// CommunitySetID identifies the hierarchy containing CommunityID.
	CommunitySetID string `json:"community_set_id"`
	// CorporaID identifies the evidence collection containing source TextUnits.
	CorporaID string `json:"corpora_id"`
	// ID is the immutable Report UUID requested by the client.
	ID string `json:"id"`
	// CommunityID is the immutable summarized Entity-membership identity.
	CommunityID string `json:"community_id"`
	// CommunityNumber is the CommunitySet-local non-negative display ordinal.
	CommunityNumber int `json:"community_number"`
	// Level is the hierarchy depth, where roots use zero.
	Level int `json:"level"`
	// ParentID is absent for a root and otherwise identifies its direct parent.
	ParentID *string `json:"parent_id,omitempty"`
	// Children contains direct child Community IDs in Community number order.
	Children []string `json:"children"`
	// Title is the persisted model-generated Report title.
	Title string `json:"title"`
	// Summary is the persisted model-generated executive summary.
	Summary string `json:"summary"`
	// FullContent is the persisted deterministic Markdown projection.
	FullContent string `json:"full_content"`
	// Rank is the finite model-generated impact rating.
	Rank float64 `json:"rank"`
	// RatingExplanation is the persisted model explanation for Rank.
	RatingExplanation string `json:"rating_explanation"`
	// Findings preserves model order and is always encoded as a JSON array.
	Findings []httpReportFinding `json:"findings"`
	// Period is the business period fixed when the Report was generated.
	Period string `json:"period"`
	// Size is the number of Entity IDs in the exact Community membership.
	Size int `json:"size"`
	// Sources contains exact historical Knowledge references and Corpus evidence IDs.
	Sources httpReportSources `json:"sources"`
}

// httpReportFinding is one model-produced Report finding in persisted order.
type httpReportFinding struct {
	// Summary is the required short finding title.
	Summary string `json:"summary"`
	// Explanation is the required supporting narrative.
	Explanation string `json:"explanation"`
}

// httpReportSources is the complete identity-only evidence closure recorded by
// Report generation; each collection is always encoded as a JSON array.
type httpReportSources struct {
	// Entities preserves exact Entity version references in model-input order.
	Entities []httpKnowledgeReference `json:"entities"`
	// Relations preserves exact directed Relation versions in model-input order.
	Relations []httpKnowledgeReference `json:"relations"`
	// Claims preserves exact Claim Statement references in model-input order.
	Claims []httpClaimReference `json:"claims"`
	// TextUnitIDs is the sorted, duplicate-free evidence set in CorporaID.
	TextUnitIDs []string `json:"text_unit_ids"`
}

// httpKnowledgeReference identifies one exact immutable Entity or Relation row;
// the containing JSON field selects its knowledge type.
type httpKnowledgeReference struct {
	// ID is the permanent logical Knowledge identity.
	ID string `json:"id"`
	// Version is positive and only comparable inside ID's version chain.
	Version uint64 `json:"version"`
}

// httpClaimReference identifies one Statement inside an exact immutable Claim
// version without copying statement content into the report response.
type httpClaimReference struct {
	// ID is the permanent logical Claim identity.
	ID string `json:"id"`
	// Version is the positive immutable Claim version used by Report generation.
	Version uint64 `json:"version"`
	// EvidenceIndex is the zero-based position inside that exact Claim Version.
	EvidenceIndex int `json:"evidence_index"`
}

func newHTTPQueryResponse(method string, outcome httpQueryOutcome) httpQueryResponse {
	return httpQueryResponse{
		Method: method, EpochID: outcome.epochID,
		ReportSetID: outcome.reportSetID, CommunitySetID: outcome.communitySetID,
		CorporaID: outcome.corporaID,
		Response:  outcome.response, CitationAudit: outcome.citations,
	}
}

func newHTTPSuggestionResponse(execution queryapplication.SuggestionExecution) httpSuggestionResponse {
	return httpSuggestionResponse{
		Questions: append([]string(nil), execution.Questions...),
		EpochID:   execution.EpochID, ReportSetID: execution.ReportSetID,
		CommunitySetID: execution.CommunitySetID, CorporaID: execution.CorporaID,
	}
}

func newHTTPCitationAudit(audit querybase.CitationAudit) *httpCitationAudit {
	items := make([]httpCitation, 0, len(audit.Items))
	for _, item := range audit.Items {
		sources := make([]httpCitationSource, 0, len(item.Sources))
		for _, source := range item.Sources {
			sources = append(sources, httpCitationSource{
				CorporaID:        source.CorporaID,
				TextUnitID:       source.TextUnitID,
				Text:             source.Text,
				DocumentID:       source.DocumentID,
				DocumentLocation: source.DocumentLocation,
				TextTitle:        source.TextTitle,
			})
		}
		var recordID *int
		if item.Parsed {
			recordID = integerAddress(item.Reference.RecordID)
		}
		items = append(items, httpCitation{
			Raw: item.Raw, Dataset: string(item.Reference.Dataset),
			RecordID: recordID, Parsed: item.Parsed,
			Status: string(item.Status), InvalidReason: string(item.InvalidReason),
			Sources: sources,
		})
	}
	return &httpCitationAudit{Missing: audit.Missing, Items: items}
}

func integerAddress(value int) *int {
	return &value
}

func newHTTPReportPage(page queryreport.ReportPage) httpReportPage {
	reports := make([]httpReportSummary, 0, len(page.Reports))
	for _, report := range page.Reports {
		reports = append(reports, httpReportSummary{
			ID: report.ID, CommunityID: report.CommunityID,
			CommunityNumber: report.CommunityNumber, Level: report.Level, Title: report.Title,
			Summary: report.Summary, Rank: report.Rank, Period: report.Period, Size: report.Size,
		})
	}
	return httpReportPage{
		EpochID:     page.EpochID,
		ReportSetID: page.ReportSetID, CommunitySetID: page.CommunitySetID,
		CorporaID: page.CorporaID, Page: page.Page, PageSize: page.PageSize,
		Total: page.Total, Reports: reports,
	}
}

func newHTTPReportDetail(report queryreport.ReportDetail) httpReportDetail {
	findings := make([]httpReportFinding, 0, len(report.Findings))
	for _, finding := range report.Findings {
		findings = append(findings, httpReportFinding{Summary: finding.Summary, Explanation: finding.Explanation})
	}
	entities := make([]httpKnowledgeReference, len(report.Sources.Entities))
	for index, source := range report.Sources.Entities {
		entities[index] = httpKnowledgeReference{ID: source.ID, Version: source.Version}
	}
	relations := make([]httpKnowledgeReference, len(report.Sources.Relations))
	for index, source := range report.Sources.Relations {
		relations[index] = httpKnowledgeReference{ID: source.ID, Version: source.Version}
	}
	claims := make([]httpClaimReference, len(report.Sources.Claims))
	for index, source := range report.Sources.Claims {
		claims[index] = httpClaimReference{
			ID: source.ID, Version: source.Version, EvidenceIndex: source.EvidenceIndex,
		}
	}
	return httpReportDetail{
		EpochID:     report.EpochID,
		ReportSetID: report.ReportSetID, CommunitySetID: report.CommunitySetID,
		CorporaID: report.CorporaID, ID: report.ID,
		CommunityID: report.CommunityID, CommunityNumber: report.CommunityNumber,
		Level: report.Level, ParentID: report.ParentID,
		Children: nonNilHTTPList(report.Children), Title: report.Title, Summary: report.Summary,
		FullContent: report.FullContent, Rank: report.Rank, RatingExplanation: report.RatingExplanation,
		Findings: findings, Period: report.Period, Size: report.Size,
		Sources: httpReportSources{
			Entities: nonNilHTTPList(entities), Relations: nonNilHTTPList(relations),
			Claims: nonNilHTTPList(claims), TextUnitIDs: nonNilHTTPList(report.Sources.TextUnitIDs),
		},
	}
}

// nonNilHTTPList preserves the API contract that collection fields are JSON
// arrays even when the domain result has no members. It also prevents response
// DTOs from sharing mutable slice storage with application results.
func nonNilHTTPList[T any](values []T) []T {
	result := make([]T, len(values))
	copy(result, values)
	return result
}
