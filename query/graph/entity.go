package graph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (c *Catalog) BrowseEntities(
	ctx context.Context,
	request EntityGraphRequest,
) (_ EntityGraph, resultErr error) {
	request, err := normalizeEntityRequest(request)
	if err != nil {
		return EntityGraph{}, err
	}
	if c == nil || c.knowledge == nil {
		return EntityGraph{}, errors.New("graph Catalog is required")
	}
	view, err := c.knowledge.OpenCurrent(ctx)
	if err != nil {
		return EntityGraph{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	entities, err := readEntities(ctx, view)
	if err != nil {
		return EntityGraph{}, err
	}
	relations, err := readRelations(ctx, view)
	if err != nil {
		return EntityGraph{}, err
	}
	matched := make([]KnowledgeEntity, 0, len(entities))
	for _, entity := range entities {
		if entity.Deleted || !matchesEntity(entity, request) {
			continue
		}
		matched = append(matched, entity)
	}
	sort.Slice(matched, func(left, right int) bool {
		if matched[left].Degree != matched[right].Degree {
			return matched[left].Degree > matched[right].Degree
		}
		if matched[left].Title != matched[right].Title {
			return matched[left].Title < matched[right].Title
		}
		return matched[left].ID < matched[right].ID
	})
	matchedEntityCount := len(matched)
	if len(matched) > request.Limit {
		matched = matched[:request.Limit]
	}
	selected := make(map[string]struct{}, len(matched))
	resultEntities := make([]Entity, 0, len(matched))
	for _, entity := range matched {
		selected[entity.ID] = struct{}{}
		resultEntities = append(resultEntities, projectEntity(entity))
	}
	selectedRelations := make([]KnowledgeRelation, 0)
	for _, relation := range relations {
		if relation.Deleted {
			continue
		}
		_, source := selected[relation.SourceEntityID]
		_, target := selected[relation.TargetEntityID]
		if source && target {
			selectedRelations = append(selectedRelations, relation)
		}
	}
	sortRelations(selectedRelations)
	matchedRelationCount := len(selectedRelations)
	if len(selectedRelations) > MaximumRelationLimit {
		selectedRelations = selectedRelations[:MaximumRelationLimit]
	}
	resultRelations := make([]Relation, 0, len(selectedRelations))
	for _, relation := range selectedRelations {
		resultRelations = append(resultRelations, projectRelation(relation))
	}
	return EntityGraph{
		Entities: resultEntities, Relations: resultRelations,
		MatchedEntities: matchedEntityCount, MatchedRelations: matchedRelationCount,
		Truncated: matchedEntityCount > len(resultEntities) || matchedRelationCount > len(resultRelations),
	}, nil
}

func (c *Catalog) Neighborhood(
	ctx context.Context,
	request NeighborhoodRequest,
) (_ Neighborhood, resultErr error) {
	request.EntityID = strings.TrimSpace(request.EntityID)
	if request.EntityID == "" {
		return Neighborhood{}, fmt.Errorf("%w: Entity ID is required", ErrInvalidRequest)
	}
	if request.RelationLimit == 0 {
		request.RelationLimit = DefaultEntityLimit
	}
	if request.RelationLimit < 1 || request.RelationLimit > MaximumEntityLimit {
		return Neighborhood{}, fmt.Errorf(
			"%w: relation limit must be between 1 and %d", ErrInvalidRequest, MaximumEntityLimit,
		)
	}
	if c == nil || c.knowledge == nil {
		return Neighborhood{}, errors.New("graph Catalog is required")
	}
	view, err := c.knowledge.OpenCurrent(ctx)
	if err != nil {
		return Neighborhood{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	entities, err := readEntities(ctx, view)
	if err != nil {
		return Neighborhood{}, err
	}
	entityByID := make(map[string]KnowledgeEntity, len(entities))
	for _, entity := range entities {
		if !entity.Deleted {
			entityByID[entity.ID] = entity
		}
	}
	center, found := entityByID[request.EntityID]
	if !found {
		return Neighborhood{}, ErrEntityNotFound
	}
	relations, err := readRelations(ctx, view)
	if err != nil {
		return Neighborhood{}, err
	}
	matched := make([]KnowledgeRelation, 0)
	for _, relation := range relations {
		if !relation.Deleted &&
			(relation.SourceEntityID == request.EntityID || relation.TargetEntityID == request.EntityID) {
			matched = append(matched, relation)
		}
	}
	sortRelations(matched)
	matchedRelationCount := len(matched)
	if len(matched) > request.RelationLimit {
		matched = matched[:request.RelationLimit]
	}
	neighborIDs := make(map[string]struct{})
	for _, relation := range matched {
		neighborIDs[relation.SourceEntityID] = struct{}{}
		neighborIDs[relation.TargetEntityID] = struct{}{}
	}
	delete(neighborIDs, request.EntityID)
	neighbors := make([]KnowledgeEntity, 0, len(neighborIDs))
	for id := range neighborIDs {
		if entity, ok := entityByID[id]; ok {
			neighbors = append(neighbors, entity)
		}
	}
	sort.Slice(neighbors, func(left, right int) bool {
		if neighbors[left].Degree != neighbors[right].Degree {
			return neighbors[left].Degree > neighbors[right].Degree
		}
		return neighbors[left].ID < neighbors[right].ID
	})
	resultEntities := make([]Entity, 0, len(neighbors)+1)
	resultEntities = append(resultEntities, projectEntity(center))
	for _, entity := range neighbors {
		resultEntities = append(resultEntities, projectEntity(entity))
	}
	resultRelations := make([]Relation, 0, len(matched))
	for _, relation := range matched {
		resultRelations = append(resultRelations, projectRelation(relation))
	}
	return Neighborhood{
		Center:   resultEntities[0],
		Entities: resultEntities, Relations: resultRelations, MatchedRelations: matchedRelationCount,
		Truncated: matchedRelationCount > len(resultRelations),
	}, nil
}

func normalizeEntityRequest(request EntityGraphRequest) (EntityGraphRequest, error) {
	request.Query = strings.TrimSpace(request.Query)
	request.EntityType = strings.TrimSpace(request.EntityType)
	if request.Limit == 0 {
		request.Limit = DefaultEntityLimit
	}
	if request.Limit < 1 || request.Limit > MaximumEntityLimit {
		return EntityGraphRequest{}, fmt.Errorf(
			"%w: entity limit must be between 1 and %d", ErrInvalidRequest, MaximumEntityLimit,
		)
	}
	return request, nil
}

func matchesEntity(entity KnowledgeEntity, request EntityGraphRequest) bool {
	if request.EntityType != "" && entity.Type != request.EntityType {
		return false
	}
	if request.Query == "" {
		return true
	}
	needle := strings.ToLower(request.Query)
	if strings.Contains(strings.ToLower(entity.Title), needle) ||
		strings.Contains(strings.ToLower(entity.Description), needle) {
		return true
	}
	for _, alias := range entity.Aliases {
		if strings.Contains(strings.ToLower(alias), needle) {
			return true
		}
	}
	return false
}

func readEntities(ctx context.Context, view KnowledgeView) ([]KnowledgeEntity, error) {
	result := make([]KnowledgeEntity, 0)
	after := ""
	for {
		page, err := view.Entities(ctx, after, knowledgeReadPageSize)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if !page.HasMore {
			return result, nil
		}
		if page.NextAfter == "" || page.NextAfter == after {
			return nil, fmt.Errorf("read Knowledge Entity page after %q: provider returned an invalid next cursor", after)
		}
		after = page.NextAfter
	}
}

func readRelations(ctx context.Context, view KnowledgeView) ([]KnowledgeRelation, error) {
	result := make([]KnowledgeRelation, 0)
	after := ""
	for {
		page, err := view.Relations(ctx, after, knowledgeReadPageSize)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if !page.HasMore {
			return result, nil
		}
		if page.NextAfter == "" || page.NextAfter == after {
			return nil, fmt.Errorf("read Knowledge Relation page after %q: provider returned an invalid next cursor", after)
		}
		after = page.NextAfter
	}
}

func projectEntity(entity KnowledgeEntity) Entity {
	return Entity{
		ID: entity.ID, Version: entity.Version, Title: entity.Title, Type: entity.Type,
		Aliases: append([]string(nil), entity.Aliases...), Description: entity.Description,
		Degree: entity.Degree, TextUnitCount: entity.TextUnitCount,
	}
}

func projectRelation(relation KnowledgeRelation) Relation {
	return Relation{
		ID: relation.ID, Version: relation.Version,
		SourceEntityID: relation.SourceEntityID, TargetEntityID: relation.TargetEntityID,
		Type: relation.Type, Description: relation.Description, Weight: relation.Weight,
		CombinedDegree: relation.CombinedDegree, TextUnitCount: relation.TextUnitCount,
	}
}

func sortRelations(relations []KnowledgeRelation) {
	sort.Slice(relations, func(left, right int) bool {
		if relations[left].CombinedDegree != relations[right].CombinedDegree {
			return relations[left].CombinedDegree > relations[right].CombinedDegree
		}
		if relations[left].Weight != relations[right].Weight {
			return relations[left].Weight > relations[right].Weight
		}
		return relations[left].ID < relations[right].ID
	})
}
