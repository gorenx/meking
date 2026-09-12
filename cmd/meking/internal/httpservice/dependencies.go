package httpservice

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/journal"
	querybase "github.com/memoria-space/meking/query"
	queryapplication "github.com/memoria-space/meking/query/application"
	querygraph "github.com/memoria-space/meking/query/graph"
	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	queryreport "github.com/memoria-space/meking/query/report"
	"github.com/memoria-space/meking/zone"
)

const (
	queryMethodBasic  = string(queryapplication.MethodBasic)
	queryMethodLocal  = string(queryapplication.MethodLocal)
	queryMethodGlobal = string(queryapplication.MethodGlobal)
	queryMethodDRIFT  = string(queryapplication.MethodDRIFT)
)

// queryRunners contains the synchronous and streaming forms of the single
// delivery contract shared by every HTTP Query method.
type queryRunners struct {
	run func(
		context.Context,
		queryapplication.Request,
	) (queryapplication.Execution, error)
	stream func(
		context.Context,
		queryapplication.Request,
		querybase.TextDeltaHandler,
	) (queryapplication.Execution, error)
}

type suggestionRunner func(
	context.Context,
	queryapplication.SuggestionRequest,
) (queryapplication.SuggestionExecution, error)

// graphBrowser is the one application aggregate exposed to HTTP graph routes.
// Delivery does not flatten its three related use cases into independent
// callbacks because they share one Epoch and Knowledge publication contract.
type graphBrowser interface {
	BrowseEntities(context.Context, querygraph.EntityGraphRequest) (querygraph.EntityGraph, error)
	Neighborhood(context.Context, querygraph.NeighborhoodRequest) (querygraph.Neighborhood, error)
	BrowseCommunities(context.Context, querygraph.CommunityRequest) (querygraph.CommunityPage, error)
}

type knowledgeBrowser interface {
	BrowseEntities(context.Context, queryknowledge.PageRequest) (queryknowledge.EntityPage, error)
	BrowseRelations(context.Context, queryknowledge.PageRequest) (queryknowledge.RelationPage, error)
	BrowseClaims(context.Context, queryknowledge.PageRequest) (queryknowledge.ClaimPage, error)
	Entity(context.Context, string, int64) (queryknowledge.EntityDetail, error)
	Relation(context.Context, string, int64) (queryknowledge.RelationDetail, error)
}

// httpDependencies contains delivery-facing callbacks consumed by HTTP route
// handlers. Tests replace these callbacks through the same production boundary.
type httpDependencies struct {
	zones       zoneDefinitionResolver
	zoneCatalog zoneCatalog
	// currentReports fixes Current Epoch and loads its exact ReportSet for readiness.
	currentReports func(context.Context) (queryreport.View, error)
	// browseReports executes one ReportSet-fixed page.
	browseReports func(context.Context, queryreport.ReportPageRequest) (queryreport.ReportPage, error)
	// readReport resolves one immutable Report ID inside an exact Epoch's ReportSet.
	readReport         func(context.Context, string, int64) (queryreport.ReportDetail, error)
	submitDocument     func(context.Context, document.UploadCommand) (corpus.DocumentReceipt, error)
	browseDocuments    func(context.Context, document.Page) (corpus.DocumentCatalogPage, error)
	listActionStatuses func(context.Context) ([]controlapplication.ActionStatus, error)
	readActionStatus   func(context.Context, controlplane.Action) (controlapplication.ActionStatus, error)
	invokeAction       func(context.Context, controlplane.Action) (controlapplication.Invocation, error)
	listPolicies       func(context.Context) ([]controlplane.Policy, error)
	readPolicy         func(context.Context, controlplane.Action) (controlplane.Policy, error)
	publishPolicy      func(
		context.Context,
		controlapplication.PublishPolicyInput,
	) (controlplane.Policy, error)
	readJournalEvents  func(context.Context, uint64, int) ([]journal.Event, error)
	readStreamEvents   func(context.Context, journal.StreamID, journal.StreamSequence, int) ([]journal.Event, error)
	graphs             graphBrowser
	knowledge          knowledgeBrowser
	queries            queryRunners
	suggestQuestions   suggestionRunner
	memoryProtocol     http.Handler
	streamPingInterval time.Duration
}

func defaultHTTPDependencies() httpDependencies {
	return httpDependencies{
		zones:       unavailableZoneResolver{},
		zoneCatalog: unavailableZoneCatalog{},
		currentReports: func(context.Context) (queryreport.View, error) {
			return queryreport.View{}, errors.New("Report Query is not configured")
		},
		browseReports: func(context.Context, queryreport.ReportPageRequest) (queryreport.ReportPage, error) {
			return queryreport.ReportPage{}, errors.New("Report Query is not configured")
		},
		readReport: func(context.Context, string, int64) (queryreport.ReportDetail, error) {
			return queryreport.ReportDetail{}, errors.New("Report Query is not configured")
		},
		submitDocument: func(context.Context, document.UploadCommand) (corpus.DocumentReceipt, error) {
			return corpus.DocumentReceipt{}, errors.New("Document upload is not configured")
		},
		browseDocuments: func(context.Context, document.Page) (corpus.DocumentCatalogPage, error) {
			return corpus.DocumentCatalogPage{}, errors.New("Document Catalog is not configured")
		},
		listActionStatuses: func(context.Context) ([]controlapplication.ActionStatus, error) {
			return nil, errors.New("Control Plane Action Statuses are not configured")
		},
		readActionStatus: func(context.Context, controlplane.Action) (controlapplication.ActionStatus, error) {
			return controlapplication.ActionStatus{}, errors.New("Control Plane Action Statuses are not configured")
		},
		invokeAction: func(context.Context, controlplane.Action) (controlapplication.Invocation, error) {
			return controlapplication.Invocation{}, errors.New("Control Plane Actions are not configured")
		},
		listPolicies: func(context.Context) ([]controlplane.Policy, error) {
			return nil, errors.New("Control Plane Policies are not configured")
		},
		readPolicy: func(context.Context, controlplane.Action) (controlplane.Policy, error) {
			return controlplane.Policy{}, errors.New("Control Plane Policies are not configured")
		},
		publishPolicy: func(
			context.Context,
			controlapplication.PublishPolicyInput,
		) (controlplane.Policy, error) {
			return controlplane.Policy{}, errors.New("Control Plane Policies are not configured")
		},
		readJournalEvents: func(context.Context, uint64, int) ([]journal.Event, error) {
			return nil, errors.New("Journal event reading is not configured")
		},
		readStreamEvents: func(context.Context, journal.StreamID, journal.StreamSequence, int) ([]journal.Event, error) {
			return nil, errors.New("Journal Stream reading is not configured")
		},
		graphs:    unavailableGraphBrowser{},
		knowledge: unavailableKnowledgeBrowser{},
		queries: queryRunners{
			run: func(context.Context, queryapplication.Request) (queryapplication.Execution, error) {
				return queryapplication.Execution{}, errors.New("Query is not configured")
			},
			stream: func(context.Context, queryapplication.Request, querybase.TextDeltaHandler) (queryapplication.Execution, error) {
				return queryapplication.Execution{}, errors.New("Query is not configured")
			},
		},
		suggestQuestions: func(context.Context, queryapplication.SuggestionRequest) (queryapplication.SuggestionExecution, error) {
			return queryapplication.SuggestionExecution{}, errors.New("Question Generation is not configured")
		},
		streamPingInterval: 15 * time.Second,
	}
}

func validateHTTPDependencies(dependencies httpDependencies) error {
	if dependencies.zones == nil || dependencies.zoneCatalog == nil ||
		dependencies.currentReports == nil || dependencies.browseReports == nil ||
		dependencies.readReport == nil || dependencies.submitDocument == nil || dependencies.browseDocuments == nil ||
		dependencies.listActionStatuses == nil || dependencies.readActionStatus == nil || dependencies.invokeAction == nil ||
		dependencies.listPolicies == nil || dependencies.readPolicy == nil ||
		dependencies.publishPolicy == nil ||
		dependencies.readJournalEvents == nil ||
		dependencies.readStreamEvents == nil ||
		dependencies.graphs == nil || dependencies.knowledge == nil {
		return errors.New("HTTP application dependencies are incomplete")
	}
	if dependencies.queries.run == nil || dependencies.queries.stream == nil {
		return errors.New("HTTP query dependencies are incomplete")
	}
	if dependencies.suggestQuestions == nil {
		return errors.New("HTTP Question Generation dependency is incomplete")
	}
	return nil
}

type unavailableZoneResolver struct{}

func (unavailableZoneResolver) Resolve(context.Context, zone.ID) (zone.Definition, error) {
	return zone.Definition{}, errors.New("Zone resolution is not configured")
}

type zoneCatalog interface {
	CreateRoot(context.Context) (zone.Definition, error)
	CreateChild(context.Context, zone.ID) (zone.Definition, error)
	List(context.Context) ([]zone.Definition, error)
}

type unavailableZoneCatalog struct{}

func (unavailableZoneCatalog) CreateRoot(context.Context) (zone.Definition, error) {
	return zone.Definition{}, errors.New("Zone creation is not configured")
}

func (unavailableZoneCatalog) CreateChild(context.Context, zone.ID) (zone.Definition, error) {
	return zone.Definition{}, errors.New("Zone creation is not configured")
}

func (unavailableZoneCatalog) List(context.Context) ([]zone.Definition, error) {
	return nil, errors.New("Zone listing is not configured")
}

type unavailableKnowledgeBrowser struct{}

func (unavailableKnowledgeBrowser) BrowseEntities(
	context.Context,
	queryknowledge.PageRequest,
) (queryknowledge.EntityPage, error) {
	return queryknowledge.EntityPage{}, errors.New("Knowledge browsing is not configured")
}

func (unavailableKnowledgeBrowser) BrowseRelations(
	context.Context,
	queryknowledge.PageRequest,
) (queryknowledge.RelationPage, error) {
	return queryknowledge.RelationPage{}, errors.New("Knowledge browsing is not configured")
}

func (unavailableKnowledgeBrowser) BrowseClaims(
	context.Context,
	queryknowledge.PageRequest,
) (queryknowledge.ClaimPage, error) {
	return queryknowledge.ClaimPage{}, errors.New("Knowledge browsing is not configured")
}

func (unavailableKnowledgeBrowser) Entity(
	context.Context,
	string,
	int64,
) (queryknowledge.EntityDetail, error) {
	return queryknowledge.EntityDetail{}, errors.New("Knowledge browsing is not configured")
}

func (unavailableKnowledgeBrowser) Relation(
	context.Context,
	string,
	int64,
) (queryknowledge.RelationDetail, error) {
	return queryknowledge.RelationDetail{}, errors.New("Knowledge browsing is not configured")
}

type unavailableGraphBrowser struct{}

func (unavailableGraphBrowser) BrowseEntities(
	context.Context,
	querygraph.EntityGraphRequest,
) (querygraph.EntityGraph, error) {
	return querygraph.EntityGraph{}, errors.New("graph browsing is not configured")
}

func (unavailableGraphBrowser) Neighborhood(
	context.Context,
	querygraph.NeighborhoodRequest,
) (querygraph.Neighborhood, error) {
	return querygraph.Neighborhood{}, errors.New("graph browsing is not configured")
}

func (unavailableGraphBrowser) BrowseCommunities(
	context.Context,
	querygraph.CommunityRequest,
) (querygraph.CommunityPage, error) {
	return querygraph.CommunityPage{}, errors.New("graph browsing is not configured")
}
