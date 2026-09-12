package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
	querysource "github.com/memoria-space/meking/query/source"
)

// Catalog browses active Knowledge at one explicit Epoch boundary and combines
// object details with evidence from that Epoch's exact Corpora.
type Catalog struct {
	epochs  EpochReader
	reader  Reader
	sources querysource.Reader
}

type EpochReader interface {
	querybase.EpochReader
	Epoch(ctx context.Context, id int64) (querybase.Epoch, error)
}

func NewCatalog(epochs EpochReader, reader Reader, sources querysource.Reader) (*Catalog, error) {
	if epochs == nil {
		return nil, errors.New("create Knowledge Catalog: Epoch reader is required")
	}
	if reader == nil {
		return nil, errors.New("create Knowledge Catalog: Knowledge reader is required")
	}
	if sources == nil {
		return nil, errors.New("create Knowledge Catalog: TextUnit source reader is required")
	}
	return &Catalog{epochs: epochs, reader: reader, sources: sources}, nil
}

func (c *Catalog) Entity(ctx context.Context, id string, epochID int64) (_ EntityDetail, resultErr error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return EntityDetail{}, fmt.Errorf("%w: Entity ID is required", ErrInvalidRequest)
	}
	epoch, view, err := c.openAt(ctx, epochID)
	if err != nil {
		return EntityDetail{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	entities, err := readAll(ctx, view.Entities)
	if err != nil {
		return EntityDetail{}, err
	}
	var selected *Entity
	for index := range entities {
		if entities[index].ID == id {
			selected = &entities[index]
		}
	}
	if selected == nil {
		return EntityDetail{}, ErrEntityNotFound
	}
	relations, err := readAll(ctx, view.Relations)
	if err != nil {
		return EntityDetail{}, err
	}
	related := make([]Relation, 0)
	neighborIDs := make(map[string]struct{})
	for _, relation := range relations {
		if relation.SourceEntityID != id && relation.TargetEntityID != id {
			continue
		}
		related = append(related, relation)
		neighborIDs[relation.SourceEntityID] = struct{}{}
		neighborIDs[relation.TargetEntityID] = struct{}{}
	}
	delete(neighborIDs, id)
	neighbors := make([]Entity, 0, len(neighborIDs))
	for _, entity := range entities {
		if _, related := neighborIDs[entity.ID]; related {
			neighbors = append(neighbors, entity)
			delete(neighborIDs, entity.ID)
		}
	}
	if len(neighborIDs) != 0 {
		return EntityDetail{}, errors.New("Entity association contains a missing active endpoint")
	}
	claims, err := relatedClaims(ctx, view, id)
	if err != nil {
		return EntityDetail{}, err
	}
	textUnits, err := c.readTextUnits(ctx, epoch.CorporaID, selected.TextUnitIDs)
	if err != nil {
		return EntityDetail{}, err
	}
	return EntityDetail{
		Publication: publication(epoch), Entity: *selected,
		Relations: related, Neighbors: neighbors, Claims: claims, TextUnits: textUnits,
	}, nil
}

func (c *Catalog) Relation(ctx context.Context, id string, epochID int64) (_ RelationDetail, resultErr error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return RelationDetail{}, fmt.Errorf("%w: Relation ID is required", ErrInvalidRequest)
	}
	epoch, view, err := c.openAt(ctx, epochID)
	if err != nil {
		return RelationDetail{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	relations, err := readAll(ctx, view.Relations)
	if err != nil {
		return RelationDetail{}, err
	}
	var selected *Relation
	for index := range relations {
		if relations[index].ID == id {
			selected = &relations[index]
			break
		}
	}
	if selected == nil {
		return RelationDetail{}, ErrRelationNotFound
	}
	entities, err := readAll(ctx, view.Entities)
	if err != nil {
		return RelationDetail{}, err
	}
	endpoints := make([]Entity, 0, 2)
	for _, entity := range entities {
		if entity.ID == selected.SourceEntityID || entity.ID == selected.TargetEntityID {
			endpoints = append(endpoints, entity)
		}
	}
	expectedEndpoints := 2
	if selected.SourceEntityID == selected.TargetEntityID {
		expectedEndpoints = 1
	}
	if len(endpoints) != expectedEndpoints {
		return RelationDetail{}, errors.New("Relation contains a missing active endpoint")
	}
	claims, err := relatedClaims(ctx, view, id)
	if err != nil {
		return RelationDetail{}, err
	}
	textUnits, err := c.readTextUnits(ctx, epoch.CorporaID, selected.TextUnitIDs)
	if err != nil {
		return RelationDetail{}, err
	}
	return RelationDetail{
		Publication: publication(epoch), Relation: *selected,
		Endpoints: endpoints, Claims: claims, TextUnits: textUnits,
	}, nil
}

func (c *Catalog) readTextUnits(
	ctx context.Context,
	corporaID string,
	ids []string,
) ([]querysource.TextUnitSource, error) {
	values, err := c.sources.Read(ctx, corporaID, ids)
	if err != nil {
		return nil, err
	}
	found := make(map[string]struct{}, len(ids))
	for _, value := range values {
		found[value.TextUnitID] = struct{}{}
	}
	for _, id := range ids {
		if _, exists := found[id]; !exists {
			return nil, errors.New("Knowledge evidence is missing from the published Corpora")
		}
	}
	return values, nil
}

func relatedClaims(ctx context.Context, view View, subjectID string) ([]Claim, error) {
	claims, err := readAll(ctx, view.Claims)
	if err != nil {
		return nil, err
	}
	result := make([]Claim, 0)
	for _, claim := range claims {
		if claim.SubjectID == subjectID {
			result = append(result, claim)
		}
	}
	return result, nil
}

func readAll[T any](
	ctx context.Context,
	read func(context.Context, string, int) (Page[T], error),
) ([]T, error) {
	result := make([]T, 0)
	after := ""
	for {
		page, err := read(ctx, after, MaximumPageSize)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if !page.HasMore {
			return result, nil
		}
		if page.NextAfter == "" || page.NextAfter == after {
			return nil, errors.New("Knowledge page did not advance")
		}
		after = page.NextAfter
	}
}

func (c *Catalog) BrowseEntities(
	ctx context.Context,
	request PageRequest,
) (_ EntityPage, resultErr error) {
	request, err := normalizePageRequest(request)
	if err != nil {
		return EntityPage{}, err
	}
	epoch, view, err := c.open(ctx)
	if err != nil {
		return EntityPage{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	page, err := view.Entities(ctx, request.After, request.Limit)
	if err != nil {
		return EntityPage{}, err
	}
	return EntityPage{
		Publication: publication(epoch), Items: nonNil(page.Items),
		NextAfter: page.NextAfter, HasMore: page.HasMore,
	}, nil
}

func (c *Catalog) BrowseRelations(
	ctx context.Context,
	request PageRequest,
) (_ RelationPage, resultErr error) {
	request, err := normalizePageRequest(request)
	if err != nil {
		return RelationPage{}, err
	}
	epoch, view, err := c.open(ctx)
	if err != nil {
		return RelationPage{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	page, err := view.Relations(ctx, request.After, request.Limit)
	if err != nil {
		return RelationPage{}, err
	}
	return RelationPage{
		Publication: publication(epoch), Items: nonNil(page.Items),
		NextAfter: page.NextAfter, HasMore: page.HasMore,
	}, nil
}

func (c *Catalog) BrowseClaims(
	ctx context.Context,
	request PageRequest,
) (_ ClaimPage, resultErr error) {
	request, err := normalizePageRequest(request)
	if err != nil {
		return ClaimPage{}, err
	}
	epoch, view, err := c.open(ctx)
	if err != nil {
		return ClaimPage{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	page, err := view.Claims(ctx, request.After, request.Limit)
	if err != nil {
		return ClaimPage{}, err
	}
	return ClaimPage{
		Publication: publication(epoch), Items: nonNil(page.Items),
		NextAfter: page.NextAfter, HasMore: page.HasMore,
	}, nil
}

func (c *Catalog) open(ctx context.Context) (querybase.Epoch, View, error) {
	if c == nil || c.epochs == nil || c.reader == nil || c.sources == nil {
		return querybase.Epoch{}, nil, errors.New("Knowledge Catalog is required")
	}
	epoch, err := c.epochs.Current(ctx)
	if errors.Is(err, querybase.ErrNoEpoch) {
		return querybase.Epoch{}, nil, ErrNoPublication
	}
	if err != nil {
		return querybase.Epoch{}, nil, err
	}
	view, err := c.reader.Open(ctx, epoch.Knowledge)
	if err != nil {
		return querybase.Epoch{}, nil, err
	}
	return epoch, view, nil
}

func (c *Catalog) openAt(ctx context.Context, epochID int64) (querybase.Epoch, View, error) {
	if c == nil || c.epochs == nil || c.reader == nil || c.sources == nil {
		return querybase.Epoch{}, nil, errors.New("Knowledge Catalog is required")
	}
	if epochID <= 0 {
		return querybase.Epoch{}, nil, fmt.Errorf("%w: Epoch ID must be positive", ErrInvalidRequest)
	}
	epoch, err := c.epochs.Epoch(ctx, epochID)
	if err != nil {
		return querybase.Epoch{}, nil, err
	}
	view, err := c.reader.Open(ctx, epoch.Knowledge)
	if err != nil {
		return querybase.Epoch{}, nil, err
	}
	return epoch, view, nil
}

func normalizePageRequest(request PageRequest) (PageRequest, error) {
	request.After = strings.TrimSpace(request.After)
	if request.Limit == 0 {
		request.Limit = DefaultPageSize
	}
	if request.Limit < 1 || request.Limit > MaximumPageSize {
		return PageRequest{}, fmt.Errorf(
			"%w: limit must be between 1 and %d", ErrInvalidRequest, MaximumPageSize,
		)
	}
	return request, nil
}

func publication(epoch querybase.Epoch) Publication {
	return Publication{
		EpochID:   epoch.ID,
		CorporaID: epoch.CorporaID, ReportSetID: epoch.ReportSetID,
	}
}

func nonNil[T any](values []T) []T {
	result := make([]T, len(values))
	copy(result, values)
	return result
}
