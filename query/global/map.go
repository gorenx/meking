package global

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querycitation "github.com/memoria-space/meking/query/internal/citation"
	queryprompt "github.com/memoria-space/meking/query/internal/prompt"
	"golang.org/x/sync/errgroup"
)

const (
	// DefaultMapLength is the requested word limit for one Map response.
	DefaultMapLength = 1000
	// DefaultMapConcurrency bounds simultaneous Map calls.
	DefaultMapConcurrency = 32
)

// MapModelRequest is the provider-independent two-message input for evaluating
// one rendered Report batch against the user's question.
type MapModelRequest struct {
	// SystemPrompt contains the rendered Report table and response word target.
	SystemPrompt string
	// UserPrompt is the original question applied to this batch.
	UserPrompt string
}

// MapModel evaluates one ContextChunk and returns a JSON object as text.
type MapModel interface {
	GenerateMap(ctx context.Context, request MapModelRequest) (string, error)
}

// MapCorrection is a rejected Map result returned to the same Agent together
// with the request that established its output contract.
type MapCorrection struct {
	Request MapModelRequest
	Result  string
	Reason  string
}

// MapCorrector is the optional correction capability used after Mapper rejects
// a structured result. Production Agent adapters implement this capability;
// simple deterministic models may omit it.
type MapCorrector interface {
	CorrectMap(ctx context.Context, correction MapCorrection) (string, error)
}

// MapConfig bounds the response length and concurrent model work for one Map
// phase. It does not select Reports or allocate context tokens.
type MapConfig struct {
	// MaxLength is the positive word-count instruction rendered into each prompt.
	MaxLength int
	// MaxConcurrency is the positive maximum number of simultaneous batch calls.
	MaxConcurrency int
}

// DefaultMapConfig returns the built-in bounded Map policy.
func DefaultMapConfig() MapConfig {
	return MapConfig{MaxLength: DefaultMapLength, MaxConcurrency: DefaultMapConcurrency}
}

// Validate rejects request policies that cannot execute bounded Map work.
func (c MapConfig) Validate() error {
	if c.MaxLength <= 0 {
		return errors.New("Global Map response length must be positive")
	}
	if c.MaxConcurrency <= 0 {
		return errors.New("Global Map concurrency must be positive")
	}
	return nil
}

// MapPoint is one scored intermediate conclusion that can compete for the
// final Reduce evidence budget.
type MapPoint struct {
	// Description is the intermediate conclusion and any Report citations emitted with it.
	Description string
	// Score is the prompt-defined importance score; values at or below zero do not enter Reduce.
	Score int
	// ReportIDs contains the zero-based request-local Reports explicitly cited
	// in Description. Mapper derives this slice from the model response, rejects
	// IDs outside the producing ContextChunk, and removes duplicates while
	// preserving first appearance. Reducer carries only accepted points forward,
	// allowing Searcher to construct the final Citation scope without reparsing.
	ReportIDs []int
	// BatchIndex identifies the ContextChunk that produced this point.
	BatchIndex int
	// PointIndex preserves the model's order within that batch.
	PointIndex int
}

// MapBatchStatus identifies whether one isolated batch produced usable output.
type MapBatchStatus string

const (
	MapBatchSucceeded       MapBatchStatus = "succeeded"
	MapBatchInvalidResponse MapBatchStatus = "invalid_response"
	MapBatchModelFailure    MapBatchStatus = "model_failure"
)

// MapBatchResult records one isolated model attempt so a malformed response or
// provider failure does not discard usable conclusions from sibling batches.
type MapBatchResult struct {
	// BatchIndex matches ContextChunk.Index and restores input order after concurrency.
	BatchIndex int
	// Context is the exact model-visible evidence used by this attempt.
	Context ContextChunk
	// Points includes a zero-score placeholder when this batch cannot contribute evidence.
	Points []MapPoint
	// Status distinguishes usable output from isolated parsing or provider failures.
	Status MapBatchStatus
	// Failure carries stable diagnostics only when Status is not succeeded.
	Failure *querybase.Failure
}

// MapRequest starts model evaluation after one fixed ReportSet Context has
// completed selection, batching, and prompt-row allocation.
type MapRequest struct {
	// Question is sent unchanged as the user message for every ContextChunk.
	Question string
	// Context contains all token-bounded chunks and fixed publication identities.
	Context Context
}

// MapResult is the ordered handoff from Map to Reduce. Context remains the
// single source for the ReportSet, CommunitySet, and Corpora fixed by Query.
type MapResult struct {
	// Context retains selected Reports and every model-visible evidence section.
	Context Context
	// Batches align one-for-one with Context.Chunks in input order.
	Batches []MapBatchResult
}

// RankedPoints filters non-positive scores and sorts the rest stably by score.
func (r MapResult) RankedPoints() []MapPoint {
	points := make([]MapPoint, 0)
	for batchIndex, batch := range r.Batches {
		for pointIndex, point := range batch.Points {
			if point.Score <= 0 {
				continue
			}
			point.BatchIndex = batchIndex
			point.PointIndex = pointIndex
			point.ReportIDs = append([]int(nil), point.ReportIDs...)
			points = append(points, point)
		}
	}
	sort.SliceStable(points, func(left, right int) bool {
		return points[left].Score > points[right].Score
	})
	return points
}

// Mapper evaluates Context chunks with bounded concurrency while preserving
// batch order and isolating non-cancellation failures.
type Mapper struct {
	model  MapModel
	prompt string
	config MapConfig
}

// NewMapper creates a provider-independent Map evaluator.
func NewMapper(model MapModel, prompt string, config MapConfig) (*Mapper, error) {
	if model == nil {
		return nil, errors.New("create Global Mapper: MapModel is required")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("create Global Mapper: Map prompt is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Mapper{model: model, prompt: prompt, config: config}, nil
}

// Map evaluates every ContextChunk and returns results in Context order.
func (m *Mapper) Map(ctx context.Context, request MapRequest) (MapResult, error) {
	if m == nil || m.model == nil {
		return MapResult{}, errors.New("Global Mapper is not configured")
	}
	if err := ctx.Err(); err != nil {
		return MapResult{}, err
	}
	result := MapResult{
		Context: request.Context,
		Batches: make([]MapBatchResult, len(request.Context.Chunks)),
	}
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(m.config.MaxConcurrency)
	for index := range request.Context.Chunks {
		index := index
		group.Go(func() error {
			batch, err := m.mapBatch(groupContext, index, request.Question, request.Context.Chunks[index])
			if err != nil {
				return err
			}
			result.Batches[index] = batch
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return MapResult{}, err
	}
	return result, nil
}

func (m *Mapper) mapBatch(
	ctx context.Context,
	batchIndex int,
	question string,
	chunk ContextChunk,
) (MapBatchResult, error) {
	result := MapBatchResult{
		BatchIndex: batchIndex,
		Context:    chunk,
		Status:     MapBatchSucceeded,
	}
	systemPrompt, err := queryprompt.Render(m.prompt, map[string]string{
		"context_data": chunk.Text,
		"max_length":   strconv.Itoa(m.config.MaxLength),
	})
	if err != nil {
		result.Status = MapBatchModelFailure
		result.Points = []MapPoint{{BatchIndex: batchIndex, Score: 0}}
		result.Failure = querybase.NewInvalidInputFailure("the Global Map prompt is invalid", err)
		return result, nil
	}
	response, err := m.model.GenerateMap(ctx, MapModelRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   question,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return MapBatchResult{}, err
		}
		result.Status = MapBatchModelFailure
		result.Points = []MapPoint{{BatchIndex: batchIndex, Score: 0}}
		result.Failure = mapFailure(err)
		return result, nil
	}
	points, err := m.mapPoints(response, chunk.ReportIDs)
	if err != nil {
		if corrector, supported := m.model.(MapCorrector); supported {
			response, correctionErr := corrector.CorrectMap(ctx, MapCorrection{
				Request: MapModelRequest{SystemPrompt: systemPrompt, UserPrompt: question},
				Result:  response,
				Reason:  err.Error(),
			})
			if correctionErr != nil {
				result.Status = MapBatchModelFailure
				result.Failure = mapFailure(correctionErr)
				result.Points = []MapPoint{{BatchIndex: batchIndex, Score: 0}}
				return result, nil
			}
			points, err = m.mapPoints(response, chunk.ReportIDs)
		}
	}
	if err != nil {
		result.Status = MapBatchInvalidResponse
		result.Failure = querybase.NewInvalidModelResponseFailure(err)
		result.Points = []MapPoint{{BatchIndex: batchIndex, Score: 0}}
		return result, nil
	}
	result.Points = make([]MapPoint, len(points))
	for index, point := range points {
		point.BatchIndex = batchIndex
		point.PointIndex = index
		result.Points[index] = point
	}
	return result, nil
}

func (m *Mapper) mapPoints(response string, reportIDs []int) ([]MapPoint, error) {
	points, err := parseMapResponse(response)
	if err != nil {
		return nil, err
	}
	return validateMapPoints(points, reportIDs)
}

func validateMapPoints(points []MapPoint, availableReportIDs []int) ([]MapPoint, error) {
	available := make(map[int]struct{}, len(availableReportIDs))
	for _, reportID := range availableReportIDs {
		if reportID < 0 {
			return nil, errors.New("Global Map Context contains a negative Report record ID")
		}
		if _, duplicate := available[reportID]; duplicate {
			return nil, errors.New("Global Map Context contains a duplicate Report record ID")
		}
		available[reportID] = struct{}{}
	}
	result := make([]MapPoint, len(points))
	for index, point := range points {
		if point.Score > 0 && strings.TrimSpace(point.Description) == "" {
			return nil, errors.New("positive Global Map point requires a description")
		}
		references, found, err := querycitation.ParseReferences(point.Description)
		if err != nil {
			return nil, err
		}
		seen := make(map[int]struct{}, len(references))
		point.ReportIDs = make([]int, 0, len(references))
		for _, reference := range references {
			if reference.Dataset != querybase.CitationReports {
				return nil, errors.New("Global Map point may cite only Reports")
			}
			if _, accepted := available[reference.RecordID]; !accepted {
				return nil, errors.New("Global Map point cites a Report outside its ContextChunk")
			}
			if _, duplicate := seen[reference.RecordID]; duplicate {
				continue
			}
			seen[reference.RecordID] = struct{}{}
			point.ReportIDs = append(point.ReportIDs, reference.RecordID)
		}
		if point.Score > 0 && (!found || len(point.ReportIDs) == 0) {
			return nil, errors.New("positive Global Map point requires a Report citation")
		}
		result[index] = point
	}
	return result, nil
}

func mapFailure(err error) *querybase.Failure {
	var failure *querybase.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return querybase.NewInternalFailure(err)
}

func parseMapResponse(response string) ([]MapPoint, error) {
	object, err := decodeJSONObject(response)
	if err != nil {
		return nil, err
	}
	pointsJSON, found := object["points"]
	if !found {
		return nil, errors.New("Global Map response is missing points")
	}
	if bytes.Equal(bytes.TrimSpace(pointsJSON), []byte("null")) {
		return []MapPoint{{Score: 0}}, nil
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(pointsJSON, &elements); err != nil {
		return nil, fmt.Errorf("decode Global Map points: %w", err)
	}
	if len(elements) == 0 {
		return []MapPoint{{Score: 0}}, nil
	}
	points := make([]MapPoint, 0, len(elements))
	for index, elementJSON := range elements {
		var element map[string]json.RawMessage
		if err := json.Unmarshal(elementJSON, &element); err != nil {
			return nil, fmt.Errorf("decode Global Map point %d: %w", index, err)
		}
		descriptionJSON, hasDescription := element["description"]
		scoreJSON, hasScore := element["score"]
		if !hasDescription {
			return nil, fmt.Errorf("Global Map point %d is missing description", index)
		}
		if !hasScore {
			return nil, fmt.Errorf("Global Map point %d is missing score", index)
		}
		var description string
		if err := json.Unmarshal(descriptionJSON, &description); err != nil {
			return nil, fmt.Errorf("Global Map description is not text: %w", err)
		}
		score, err := parseInteger(scoreJSON)
		if err != nil {
			return nil, err
		}
		points = append(points, MapPoint{Description: description, Score: score})
	}
	return points, nil
}
