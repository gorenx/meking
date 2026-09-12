package local

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
	querysource "github.com/memoria-space/meking/query/source"
)

const localAnswerPrompt = "Context:\n{context_data}\nFormat: {response_type}"

type localReportReader struct {
	view     queryreport.View
	err      error
	calls    int
	selected []querybase.Epoch
}

func (r *localReportReader) Publication(
	_ context.Context,
	selected querybase.Epoch,
) (queryreport.View, error) {
	r.calls++
	r.selected = append(r.selected, selected)
	view := copyLocalReportView(r.view)
	view.EpochID = selected.ID
	return view, r.err
}

type localEpochReader struct {
	epoch querybase.Epoch
	err   error
	calls int
}

func (r *localEpochReader) Current(context.Context) (querybase.Epoch, error) {
	r.calls++
	return r.epoch, r.err
}

type localVectorStore struct {
	reader   EntityVectorReader
	err      error
	calls    int
	entities [][]knowledge.Reference[knowledge.EntityID]
}

func (s *localVectorStore) Open(
	_ context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) (EntityVectorReader, error) {
	s.calls++
	s.entities = append(s.entities, append([]knowledge.Reference[knowledge.EntityID](nil), entities...))
	return s.reader, s.err
}

type localVectorReader struct {
	id        string
	model     string
	dimension int
	matches   []EntityMatch
	searches  int
	closes    int
}

func (r *localVectorReader) Close() error { r.closes++; return nil }

func (r *localVectorReader) ID() string     { return r.id }
func (r *localVectorReader) Model() string  { return r.model }
func (r *localVectorReader) Dimension() int { return r.dimension }
func (r *localVectorReader) Search(
	context.Context,
	[]float64,
	int,
) ([]EntityMatch, error) {
	r.searches++
	return append([]EntityMatch(nil), r.matches...), nil
}

type localEmbedder struct {
	model     string
	vector    []float64
	questions []string
}

func (e *localEmbedder) Model() string { return e.model }
func (e *localEmbedder) EmbedQuestion(_ context.Context, question string) ([]float64, error) {
	e.questions = append(e.questions, question)
	return append([]float64(nil), e.vector...), nil
}

type localKnowledgeReader struct {
	versions knowledge.Manifest
	request  KnowledgeRequest
	values   Knowledge
}

func (r *localKnowledgeReader) CurrentEntityReferences(
	_ context.Context,
	versions knowledge.Manifest,
	entityIDs []string,
) ([]querybase.KnowledgeReference, error) {
	r.versions = versions.Clone()
	result := make([]querybase.KnowledgeReference, 0, len(entityIDs))
	for _, id := range entityIDs {
		for _, entity := range r.values.Entities {
			if entity.ID == id {
				result = append(result, querybase.KnowledgeReference{ID: entity.ID, Version: entity.Version})
				break
			}
		}
	}
	return result, nil
}

func (r *localKnowledgeReader) Read(
	_ context.Context,
	versions knowledge.Manifest,
	request KnowledgeRequest,
) (Knowledge, error) {
	r.versions = versions.Clone()
	r.request = request
	return r.values, nil
}

type localTokenCounter struct{}

func (localTokenCounter) Count(_ context.Context, _ string, text string) (int, error) {
	return len([]rune(text)), nil
}

type localAnswerModel struct {
	request  AnswerModelRequest
	response string
	stream   []string
}

func (m *localAnswerModel) GenerateAnswer(
	_ context.Context,
	request AnswerModelRequest,
) (string, error) {
	m.request = request
	return m.response, nil
}

func (m *localAnswerModel) StreamAnswer(
	ctx context.Context,
	request AnswerModelRequest,
	emit querybase.TextDeltaHandler,
) (string, error) {
	m.request = request
	var response strings.Builder
	for _, part := range m.stream {
		if err := emit(part); err != nil {
			return response.String(), err
		}
		response.WriteString(part)
	}
	return response.String(), nil
}

type localSources struct {
	requests [][]string
}

func (s *localSources) Read(
	_ context.Context,
	corporaID string,
	textUnitIDs []string,
) ([]querysource.TextUnitSource, error) {
	s.requests = append(s.requests, append([]string(nil), textUnitIDs...))
	result := make([]querysource.TextUnitSource, len(textUnitIDs))
	for index, id := range textUnitIDs {
		result[index] = querysource.TextUnitSource{
			CorporaID: corporaID, TextUnitID: id, Text: "text " + id,
			DocumentID: "document", TextTitle: "source.txt",
		}
	}
	return result, nil
}

func TestLocalSearchFixesIndependentEntityAndReportVersions(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	epochs := &localEpochReader{epoch: localEpoch()}
	vectorReader := &localVectorReader{
		id: "entity-vector-current-4", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{{ID: "entity-a", Version: 4, Score: 0.9}},
	}
	vectors := &localVectorStore{reader: vectorReader}
	embedder := &localEmbedder{model: "embedding-model", vector: []float64{1, 0}}
	knowledge := &localKnowledgeReader{values: localKnowledge()}
	model := &localAnswerModel{response: "answer [Data: Reports (0); Entities (0); Relationships (0); Claims (0); Sources (0)]"}
	sources := &localSources{}
	searcher := newLocalSearcherForTestWithEpoch(
		t, epochs, reports, vectors, embedder, knowledge, model, sources,
	)

	result, err := searcher.Search(t.Context(), SearchRequest{Question: "How is Alpha connected?"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if epochs.calls != 1 || reports.calls != 1 || vectors.calls != 1 || vectorReader.searches != 1 ||
		!knowledge.versions.Equal(epochs.epoch.Knowledge) {
		t.Fatalf(
			"fixed reads = Epoch:%d reports:%d vectors:%d searches:%d Knowledge:%#v",
			epochs.calls, reports.calls, vectors.calls, vectorReader.searches,
			knowledge.versions,
		)
	}
	if vectorReader.closes != 1 {
		t.Fatalf("Entity vector reader closes = %d, want 1", vectorReader.closes)
	}
	if result.EpochID != 5 || result.ReportSetID != "report-set-3" || result.CommunitySetID != "community-set-3" ||
		result.CorporaID != "corpora-3" {
		t.Fatalf("fixed identities = %#v", result)
	}
	wantRequest := KnowledgeRequest{
		Entities: []querybase.KnowledgeReference{
			{ID: "entity-a", Version: 4},
			{ID: "entity-b", Version: 2},
		},
		Relations: []querybase.KnowledgeReference{{ID: "relation-ab", Version: 2}},
		Claims:    []querybase.ClaimReference{{ID: "claim-ab", Version: 1, EvidenceIndex: 0}},
	}
	if !reflect.DeepEqual(knowledge.request, wantRequest) {
		t.Fatalf("Knowledge request = %#v, want %#v", knowledge.request, wantRequest)
	}
	if !strings.Contains(result.Context.Text, "Alpha current") ||
		!strings.Contains(result.Context.Text, "# Alpha report v4") ||
		!strings.Contains(result.Context.Text, "Alpha current|Beta report|connected") ||
		!strings.Contains(result.Context.Text, "claim source") {
		t.Fatalf("Local context omitted exact evidence:\n%s", result.Context.Text)
	}
	if len(result.Context.Reports.Rows) != 1 || len(result.Context.Entities.Rows) != 1 ||
		len(result.Context.Relationships.Rows) != 1 || len(result.Context.Claims.Rows) != 1 ||
		len(result.Context.Sources.Rows) == 0 {
		t.Fatalf("context row counts = reports:%d entities:%d relationships:%d claims:%d sources:%d",
			len(result.Context.Reports.Rows), len(result.Context.Entities.Rows),
			len(result.Context.Relationships.Rows), len(result.Context.Claims.Rows),
			len(result.Context.Sources.Rows))
	}
	if len(result.CitationAudit.Items) != 5 {
		t.Fatalf("Citation items = %#v", result.CitationAudit.Items)
	}
	for _, item := range result.CitationAudit.Items {
		if item.Status != querybase.CitationValid {
			t.Fatalf("Citation = %#v, want valid", item)
		}
	}
	if !strings.Contains(model.request.SystemPrompt, result.Context.Text) ||
		model.request.UserPrompt != "How is Alpha connected?" {
		t.Fatalf("model request = %#v", model.request)
	}
}

func TestLocalSessionReusesFixedViewsAcrossEvidenceBuilds(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	epochs := &localEpochReader{epoch: localEpoch()}
	fixedVectors := &localVectorReader{
		id: "entity-vector-fixed", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{{ID: "entity-a", Version: 4, Score: 0.9}},
	}
	vectors := &localVectorStore{reader: fixedVectors}
	searcher := newLocalSearcherForTestWithEpoch(
		t, epochs, reports, vectors,
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		&localKnowledgeReader{values: localKnowledge()},
		&localAnswerModel{response: "unused"}, &localSources{},
	)

	session, err := searcher.Open(t.Context())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	epochs.epoch = querybase.Epoch{
		ID: 6, CorporaID: "later-corpora", ReportSetID: "later-report-set",
	}
	vectors.reader = &localVectorReader{
		id: "later-entity-vector", model: "embedding-model", dimension: 2,
	}
	for _, question := range []string{"first branch", "second branch"} {
		evidence, err := session.Evidence(t.Context(), SearchRequest{Question: question})
		if err != nil {
			t.Fatalf("Evidence(%q) error = %v", question, err)
		}
		if evidence.ReportSetID != "report-set-3" {
			t.Fatalf("Evidence(%q) fixed identities = %#v", question, evidence)
		}
	}
	if epochs.calls != 1 || reports.calls != 1 || vectors.calls != 1 || fixedVectors.searches != 2 {
		t.Fatalf(
			"fixed view calls = Epoch:%d reports:%d vectors:%d searches:%d",
			epochs.calls, reports.calls, vectors.calls, fixedVectors.searches,
		)
	}
	if !reflect.DeepEqual(reports.selected, []querybase.Epoch{localEpoch()}) {
		t.Fatalf("selected Epochs = %#v", reports.selected)
	}

	copy, err := session.ReportView(t.Context())
	if err != nil {
		t.Fatalf("ReportView() error = %v", err)
	}
	copy.ReportSetID = "mutated"
	copy.Communities[0].EntityIDs[0] = "mutated"
	copy.Reports[0].Sources.Entities[0].ID = "mutated"
	retained, err := session.ReportView(t.Context())
	if err != nil {
		t.Fatalf("ReportView() second error = %v", err)
	}
	if retained.ReportSetID != "report-set-3" ||
		retained.Communities[0].EntityIDs[0] != "entity-a" ||
		retained.Reports[0].Sources.Entities[0].ID != "entity-a" {
		t.Fatalf("ReportView() leaked caller mutation = %#v", retained)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Session Close() error = %v", err)
	}
	if err := session.Close(); err != nil || fixedVectors.closes != 1 {
		t.Fatalf("idempotent Session Close() = %v, vector closes = %d", err, fixedVectors.closes)
	}
	_, err = session.Evidence(t.Context(), SearchRequest{Question: "closed"})
	assertFailureCategory(t, err, querybase.FailureInternal)
}

func TestLocalSessionOpenAtUsesOwningQueryEpochWithoutReadingCurrent(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	epochs := &localEpochReader{epoch: querybase.Epoch{
		ID: 99, CorporaID: "wrong-corpus", ReportSetID: "wrong-reports",
	}}
	fixedVectors := &localVectorReader{
		id: "entity-vector-fixed", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{{ID: "entity-a", Version: 4, Score: 0.9}},
	}
	searcher := newLocalSearcherForTestWithEpoch(
		t, epochs, reports, &localVectorStore{reader: fixedVectors},
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		&localKnowledgeReader{values: localKnowledge()},
		&localAnswerModel{response: "unused"}, &localSources{},
	)

	selected := localEpoch()
	session, err := searcher.OpenAt(t.Context(), selected)
	if err != nil {
		t.Fatalf("OpenAt() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	evidence, err := session.Evidence(t.Context(), SearchRequest{Question: "branch"})
	if err != nil {
		t.Fatalf("Evidence() error = %v", err)
	}
	if epochs.calls != 0 || evidence.EpochID != selected.ID ||
		evidence.ReportSetID != selected.ReportSetID || evidence.CorporaID != selected.CorporaID {
		t.Fatalf("fixed Epoch = calls:%d evidence:%#v", epochs.calls, evidence)
	}
	if !session.Epoch().Equal(selected) {
		t.Fatalf("Session Epoch = %#v, want %#v", session.Epoch(), selected)
	}
	if !reflect.DeepEqual(reports.selected, []querybase.Epoch{selected}) {
		t.Fatalf("Report selections = %#v", reports.selected)
	}
}

func TestLocalSessionEvidenceIgnoresAnswerOnlyResponseType(t *testing.T) {
	searcher := newLocalSearcherForTest(
		t, &localReportReader{view: localReportView()},
		&localVectorStore{reader: &localVectorReader{
			id: "entity-vector", model: "embedding-model", dimension: 2,
			matches: []EntityMatch{{ID: "entity-a", Version: 4, Score: 0.9}},
		}},
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		&localKnowledgeReader{values: localKnowledge()},
		&localAnswerModel{response: "unused"}, &localSources{},
	)
	session, err := searcher.OpenAt(t.Context(), localEpoch())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if _, err := session.Evidence(t.Context(), SearchRequest{
		Question: "branch", ResponseType: " answer-only value with spaces ",
	}); err != nil {
		t.Fatalf("Evidence() validated unused ResponseType: %v", err)
	}
	_, err = session.Search(t.Context(), SearchRequest{
		Question: "branch", ResponseType: " answer-only value with spaces ",
	})
	assertFailureCategory(t, err, querybase.FailureInvalidInput)
}

func TestLocalSearchAppliesExplicitEntityFiltersOnOneVectorView(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	reader := &localVectorReader{
		id: "entity-vectors", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{
			{ID: "entity-a", Version: 4, Score: 0.9},
			{ID: "entity-b", Version: 2, Score: 0.8},
		},
	}
	knowledge := &localKnowledgeReader{values: Knowledge{
		Entities: []Entity{{
			ID: "entity-b", Version: 2, Title: "Beta", Description: "selected",
			TextUnitIDs: []string{"unit-b"}, Degree: 1,
		}},
	}}
	model := &localAnswerModel{response: "Beta [Data: Entities (0)]"}
	searcher := newLocalSearcherForTest(
		t, reports, &localVectorStore{reader: reader},
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		knowledge, model, &localSources{},
	)
	result, err := searcher.Search(t.Context(), SearchRequest{
		Question: "Beta", IncludeEntityIDs: []string{"entity-b"},
		ExcludeEntityIDs: []string{"entity-a"},
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Matches) != 1 || result.Matches[0].ID != "entity-b" {
		t.Fatalf("matches = %#v", result.Matches)
	}
}

func TestLocalSearchUsesRecentUserHistoryForEntityRetrieval(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	reader := &localVectorReader{
		id: "entity-vectors", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{{ID: "entity-a", Version: 4, Score: 0.9}},
	}
	embedder := &localEmbedder{model: "embedding-model", vector: []float64{1, 0}}
	searcher := newLocalSearcherForTest(
		t, reports, &localVectorStore{reader: reader}, embedder,
		&localKnowledgeReader{values: localKnowledge()},
		&localAnswerModel{response: "answer"}, &localSources{},
	)
	_, err := searcher.Search(t.Context(), SearchRequest{
		Question: "Current question",
		Conversation: []querybase.ConversationTurn{
			{Role: querybase.RoleUser, Content: "Old question"},
			{Role: querybase.RoleAssistant, Content: "Old answer"},
			{Role: querybase.RoleUser, Content: "Recent question"},
			{Role: querybase.RoleSystem, Content: "System note"},
		},
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !reflect.DeepEqual(embedder.questions, []string{
		"Current question\nRecent question\nOld question",
	}) {
		t.Fatalf("embedding questions = %#v", embedder.questions)
	}
}

func TestLocalSearchKeepsVectorMatchedEntityWithoutCommunity(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	reader := &localVectorReader{
		id: "entity-vectors", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{{ID: "isolated-entity", Version: 2, Score: 0.9}},
	}
	knowledge := &localKnowledgeReader{values: Knowledge{Entities: []Entity{{
		ID: "isolated-entity", Version: 2, Title: "Isolated", Description: "No community",
		TextUnitIDs: []string{"unit-isolated"},
	}}}}
	searcher := newLocalSearcherForTest(
		t, reports, &localVectorStore{reader: reader},
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		knowledge, &localAnswerModel{response: "Isolated [Data: Entities (0)]"}, &localSources{},
	)

	result, err := searcher.Search(t.Context(), SearchRequest{Question: "Isolated"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Context.Reports.Rows) != 0 || len(result.Context.Entities.Rows) != 1 ||
		result.Context.Entities.Rows[0].Values[1] != "Isolated" {
		t.Fatalf("context = %#v", result.Context)
	}
	if len(result.CitationAudit.Items) != 1 ||
		result.CitationAudit.Items[0].Status != querybase.CitationValid {
		t.Fatalf("Citation audit = %#v", result.CitationAudit)
	}
}

func TestSelectRelationshipsTreatsBothDirectionsAsIncident(t *testing.T) {
	matches := []EntityMatch{{ID: "a", Version: 1}, {ID: "b", Version: 1}}
	relationships := []Relationship{
		{ID: "out-single", SourceEntityID: "a", TargetEntityID: "d", CombinedDegree: 100},
		{ID: "in-shared-a", SourceEntityID: "c", TargetEntityID: "a", CombinedDegree: 3},
		{ID: "internal", SourceEntityID: "b", TargetEntityID: "a", CombinedDegree: 1},
		{ID: "out-shared-b", SourceEntityID: "b", TargetEntityID: "c", CombinedDegree: 2},
	}

	selected := selectRelationships(relationships, matches, 10)
	ids := make([]string, len(selected))
	for index, relationship := range selected {
		ids[index] = relationship.ID
	}
	want := []string{"internal", "in-shared-a", "out-shared-b", "out-single"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("selected Relationship IDs = %v, want %v", ids, want)
	}
}

func TestDefaultConfigMatchesLocalSearchBudgetPolicy(t *testing.T) {
	config := DefaultConfig()
	if config.CommunityProportion != 0.15 || config.TextUnitProportion != 0.5 ||
		config.MaxContextTokens != 12000 || config.TopKEntities != 10 ||
		config.TopKRelationships != 10 {
		t.Fatalf("DefaultConfig() = %#v", config)
	}
}

func TestLocalSearchReportsStableNoEvidenceAndInvalidInput(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	reader := &localVectorReader{id: "vectors", model: "embedding-model", dimension: 2}
	searcher := newLocalSearcherForTest(
		t, reports, &localVectorStore{reader: reader},
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		&localKnowledgeReader{}, &localAnswerModel{}, &localSources{},
	)
	_, err := searcher.Search(t.Context(), SearchRequest{Question: "unknown"})
	assertFailureCategory(t, err, querybase.FailureNoEvidence)
	_, err = searcher.Search(t.Context(), SearchRequest{
		Question: "conflict", IncludeEntityIDs: []string{"entity-a"},
		ExcludeEntityIDs: []string{"entity-a"},
	})
	assertFailureCategory(t, err, querybase.FailureInvalidInput)
}

func TestLocalSearchTreatsEmptyActiveEntityIndexAsNoEvidence(t *testing.T) {
	reader := &localVectorReader{id: "empty-vectors", model: "embedding-model", dimension: 0}
	embedder := &localEmbedder{model: "embedding-model", vector: []float64{1, 0}}
	searcher := newLocalSearcherForTest(
		t, &localReportReader{view: localReportView()}, &localVectorStore{reader: reader},
		embedder, &localKnowledgeReader{}, &localAnswerModel{}, &localSources{},
	)
	_, err := searcher.Search(t.Context(), SearchRequest{Question: "unknown"})
	assertFailureCategory(t, err, querybase.FailureNoEvidence)
	if len(embedder.questions) != 0 || reader.searches != 0 || reader.closes != 1 {
		t.Fatalf("empty index calls = embeds:%d searches:%d closes:%d",
			len(embedder.questions), reader.searches, reader.closes)
	}
}

func TestLocalSearchRejectsInvalidEntityVectorMatch(t *testing.T) {
	reader := &localVectorReader{
		id: "vectors", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{{ID: "entity-a", Version: 1, Score: 2}},
	}
	searcher := newLocalSearcherForTest(
		t, &localReportReader{view: localReportView()}, &localVectorStore{reader: reader},
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		&localKnowledgeReader{}, &localAnswerModel{}, &localSources{},
	)
	_, err := searcher.Search(t.Context(), SearchRequest{Question: "invalid match"})
	assertFailureCategory(t, err, querybase.FailurePublicationIncomplete)
}

func TestLocalStreamEmitsOnlyFinalAnswerAndAuditsSameContext(t *testing.T) {
	reports := &localReportReader{view: localReportView()}
	reader := &localVectorReader{
		id: "entity-vectors", model: "embedding-model", dimension: 2,
		matches: []EntityMatch{{ID: "entity-a", Version: 4, Score: 0.9}},
	}
	model := &localAnswerModel{stream: []string{"streamed ", "[Data: Entities (0)]"}}
	searcher := newLocalSearcherForTest(
		t, reports, &localVectorStore{reader: reader},
		&localEmbedder{model: "embedding-model", vector: []float64{1, 0}},
		&localKnowledgeReader{values: localKnowledge()}, model, &localSources{},
	)
	var emitted strings.Builder
	result, err := searcher.Stream(t.Context(), SearchRequest{Question: "Alpha"}, func(delta string) error {
		emitted.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if result.Response != emitted.String() || len(result.CitationAudit.Items) != 1 ||
		result.CitationAudit.Items[0].Status != querybase.CitationValid {
		t.Fatalf("stream result = %#v, emitted = %q", result, emitted.String())
	}
}

func newLocalSearcherForTest(
	t *testing.T,
	reports ReportReader,
	vectors EntityVectorStore,
	embedder QuestionEmbedder,
	knowledge KnowledgeReader,
	model AnswerModel,
	sources querysource.Reader,
) *Searcher {
	return newLocalSearcherForTestWithEpoch(
		t, &localEpochReader{epoch: localEpoch()}, reports, vectors,
		embedder, knowledge, model, sources,
	)
}

func newLocalSearcherForTestWithEpoch(
	t *testing.T,
	epochs querybase.EpochReader,
	reports ReportReader,
	vectors EntityVectorStore,
	embedder QuestionEmbedder,
	knowledge KnowledgeReader,
	model AnswerModel,
	sources querysource.Reader,
) *Searcher {
	t.Helper()
	searcher, err := NewSearcher(
		epochs, reports, vectors, embedder, knowledge, localTokenCounter{}, model, sources,
		localAnswerPrompt, DefaultConfig(),
	)
	if err != nil {
		t.Fatalf("NewSearcher() error = %v", err)
	}
	return searcher
}

func localEpoch() querybase.Epoch {
	return querybase.Epoch{
		ID: 5,
		Knowledge: knowledge.Manifest{
			Entities: []knowledge.Reference[knowledge.EntityID]{
				{ID: "entity-a", Version: 4},
				{ID: "entity-b", Version: 2},
			},
			Relations: []knowledge.Reference[knowledge.RelationID]{
				{ID: "relation-ab", Version: 2},
			},
			Claims: []knowledge.Reference[knowledge.ClaimID]{
				{ID: "claim-ab", Version: 1},
			},
		},
		CorporaID:   "corpora-3",
		ReportSetID: "report-set-3",
	}
}

func copyLocalReportView(view queryreport.View) queryreport.View {
	result := view
	result.Communities = make([]queryreport.PublishedCommunity, len(view.Communities))
	for index, community := range view.Communities {
		community.EntityIDs = append([]string(nil), community.EntityIDs...)
		result.Communities[index] = community
	}
	result.Reports = make([]queryreport.PublishedReport, len(view.Reports))
	for index, report := range view.Reports {
		report.Sources.Entities = append([]querybase.KnowledgeReference(nil), report.Sources.Entities...)
		report.Sources.Relations = append([]querybase.KnowledgeReference(nil), report.Sources.Relations...)
		report.Sources.Claims = append([]querybase.ClaimReference(nil), report.Sources.Claims...)
		report.Sources.TextUnitIDs = append([]string(nil), report.Sources.TextUnitIDs...)
		result.Reports[index] = report
	}
	return result
}

func localReportView() queryreport.View {
	return queryreport.View{
		ReportSetID: "report-set-3", CommunitySetID: "community-set-3", CorporaID: "corpora-3",
		Communities: []queryreport.PublishedCommunity{{
			ID: "community-a", Number: 0, Level: 0, EntityIDs: []string{"entity-a", "entity-b"},
		}},
		Reports: []queryreport.PublishedReport{{
			ID: "report-a", CommunityID: "community-a", Title: "Alpha report",
			FullContent: "# Alpha report v4", Rank: 8,
			Sources: queryreport.ReportSources{
				Entities: []querybase.KnowledgeReference{
					{ID: "entity-a", Version: 4}, {ID: "entity-b", Version: 2},
				},
				Relations:   []querybase.KnowledgeReference{{ID: "relation-ab", Version: 2}},
				Claims:      []querybase.ClaimReference{{ID: "claim-ab", Version: 1, EvidenceIndex: 0}},
				TextUnitIDs: []string{"unit-a-old", "unit-b", "unit-claim", "unit-relation"},
			},
		}},
	}
}

func localKnowledge() Knowledge {
	return Knowledge{
		Entities: []Entity{
			{ID: "entity-a", Version: 4, Title: "Alpha current", Description: "version four", TextUnitIDs: []string{"unit-a-current"}, Degree: 4},
			{ID: "entity-b", Version: 2, Title: "Beta report", Description: "version two", TextUnitIDs: []string{"unit-b"}, Degree: 2},
		},
		Relationships: []Relationship{{
			ID: "relation-ab", Version: 2, SourceEntityID: "entity-a", TargetEntityID: "entity-b",
			Description: "connected", Weight: 2, CombinedDegree: 5,
			TextUnitIDs: []string{"unit-relation"},
		}},
		Claims: []Claim{{
			ID: "claim-ab", Version: 1, EvidenceIndex: 0,
			Subject: RelationClaimSubject{ID: "relation-ab"}, Type: "status",
			Status: "true", Description: "claim description", SourceText: "claim source",
			TextUnitID: "unit-claim",
		}},
	}
}

func assertFailureCategory(t *testing.T, err error, want querybase.FailureCategory) {
	t.Helper()
	var failure *querybase.Failure
	if !errors.As(err, &failure) || failure.Category != want {
		t.Fatalf("error = %v, want category %s", err, want)
	}
}
