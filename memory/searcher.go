package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

const (
	defaultEntityLimit   = 10
	defaultRelationLimit = 20
	defaultClaimLimit    = 20
	defaultEvidenceLimit = 50
	maximumSearchLimit   = 200
)

type Searcher struct {
	SearchDependencies
}

func NewSearcher(dependencies SearchDependencies) (*Searcher, error) {
	switch {
	case dependencies.Knowledge == nil:
		return nil, errors.New("create Memory Searcher: Knowledge is required")
	case dependencies.Evidence == nil:
		return nil, errors.New("create Memory Searcher: Evidence Reader is required")
	case dependencies.Entities == nil:
		return nil, errors.New("create Memory Searcher: Entity Matcher is required")
	case dependencies.Corpora == nil:
		return nil, errors.New("create Memory Searcher: Corpora are required")
	case dependencies.Messages == nil:
		return nil, errors.New("create Memory Searcher: Messages are required")
	default:
		return &Searcher{SearchDependencies: dependencies}, nil
	}
}

func (searcher *Searcher) Search(ctx context.Context, query Query) (_ SearchResult, resultErr error) {
	if searcher == nil {
		return SearchResult{}, errors.New("Memory Searcher is not configured")
	}
	query, err := normalizeQuery(query)
	if err != nil {
		return SearchResult{}, err
	}
	view, err := searcher.Knowledge.OpenCurrent(ctx)
	if err != nil {
		return SearchResult{}, fmt.Errorf("open current Knowledge: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	selected, truncated, err := searcher.selectEntities(ctx, view, query)
	if err != nil {
		return SearchResult{}, err
	}
	entities, err := searcher.readEntities(ctx, view, selected)
	if err != nil {
		return SearchResult{}, err
	}
	relations, relationTruncated, err := searcher.readRelations(
		ctx,
		view,
		selected,
		query.Limits.Relations,
	)
	if err != nil {
		return SearchResult{}, err
	}
	claims, claimTruncated, err := searcher.readClaims(
		ctx,
		view,
		selected,
		relations,
		query.Limits.Claims,
	)
	if err != nil {
		return SearchResult{}, err
	}
	evidence, evidenceTruncated, err := searcher.readEvidence(
		ctx,
		entities,
		relations,
		claims,
		query.Limits.Evidence,
	)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{
		Entities:  entities,
		Relations: relations,
		Claims:    claims,
		Evidence:  evidence,
		Truncated: truncated || relationTruncated || claimTruncated || evidenceTruncated,
	}, nil
}

type entitySelection struct {
	Reference knowledge.Reference[knowledge.EntityID]
	Score     float64
	Reason    string
}

func (searcher *Searcher) selectEntities(
	ctx context.Context,
	view knowledge.View,
	query Query,
) ([]entitySelection, bool, error) {
	if !query.Fuzzy {
		page, err := view.Identities().FindEntities(
			ctx,
			query.Title,
			query.Type,
			query.Limits.Entities,
		)
		if err != nil {
			return nil, false, fmt.Errorf("find Entities by identity: %w", err)
		}
		if len(page.Items) > 0 {
			reason := "exact_title"
			if query.Type != "" {
				reason = "exact_identity"
			}
			result := make([]entitySelection, len(page.Items))
			for index, reference := range page.Items {
				result[index] = entitySelection{
					Reference: reference,
					Score:     1,
					Reason:    reason,
				}
			}
			return result, page.HasMore, nil
		}
	}

	available, err := activeReferences(ctx, view.Entities())
	if err != nil {
		return nil, false, fmt.Errorf("list current Entity references: %w", err)
	}
	if len(available) == 0 {
		return nil, false, nil
	}
	byID := make(map[knowledge.EntityID]knowledge.Reference[knowledge.EntityID], len(available))
	for _, reference := range available {
		byID[reference.ID] = reference
	}
	searchLimit := min(len(available), query.Limits.Entities)
	matches, err := searcher.Entities.MatchEntities(ctx, EntityQuery{
		Text:       entityQueryText(query),
		Candidates: available,
		Limit:      searchLimit,
	})
	if err != nil {
		return nil, false, fmt.Errorf("match current Entities: %w", err)
	}
	result := make([]entitySelection, 0, len(matches))
	selected := make(map[knowledge.EntityID]struct{}, len(matches))
	for _, match := range matches {
		reference := match.Reference
		current, found := byID[reference.ID]
		if !found || current != reference {
			return nil, false, errors.New("Entity vector result differs from current Knowledge")
		}
		if _, duplicate := selected[reference.ID]; duplicate {
			continue
		}
		result = append(result, entitySelection{
			Reference: reference,
			Score:     match.Score,
			Reason:    "semantic_match",
		})
		selected[reference.ID] = struct{}{}
	}
	return result, len(available) > len(result) && len(result) == query.Limits.Entities, nil
}

func activeReferences[T knowledge.Knowledge, ID knowledge.KnowledgeID](
	ctx context.Context,
	reader knowledge.VersionReader[T, ID],
) ([]knowledge.Reference[ID], error) {
	const pageSize = 256
	var result []knowledge.Reference[ID]
	var after ID
	for {
		page, err := reader.Active(ctx, after, pageSize)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if !page.HasMore {
			return result, nil
		}
		after = page.NextAfter
	}
}

func entityQueryText(query Query) string {
	if query.Type == "" {
		return query.Title
	}
	return query.Title + " " + query.Type
}

func (searcher *Searcher) readEntities(
	ctx context.Context,
	view knowledge.View,
	selected []entitySelection,
) ([]EntityMatch, error) {
	references := make([]knowledge.Reference[knowledge.EntityID], len(selected))
	for index, selection := range selected {
		references[index] = selection.Reference
	}
	versions, err := view.Entities().Read(ctx, references)
	if err != nil {
		return nil, err
	}
	if len(versions) != len(references) {
		return nil, errors.New("Memory Entity result is incomplete")
	}
	result := make([]EntityMatch, len(versions))
	for index, version := range versions {
		if version.Deleted || version.Version != references[index].Version || version.Knowledge.ID != references[index].ID {
			return nil, errors.New("Memory Entity result differs from current Knowledge")
		}
		evidence, err := searcher.Evidence.ReadEntityEvidence(ctx, references[index])
		if err != nil {
			return nil, err
		}
		result[index] = EntityMatch{
			Version:  version,
			Score:    selected[index].Score,
			Reason:   selected[index].Reason,
			Evidence: evidence,
		}
	}
	return result, nil
}

func (searcher *Searcher) readRelations(
	ctx context.Context,
	view knowledge.View,
	selected []entitySelection,
	limit int,
) ([]RelationMatch, bool, error) {
	entityIDs := make(map[knowledge.EntityID]struct{}, len(selected))
	for _, selection := range selected {
		entityIDs[selection.Reference.ID] = struct{}{}
	}
	result := make([]RelationMatch, 0, limit)
	for after := knowledge.RelationID(""); ; {
		page, err := view.Relations().Current(ctx, after, 256)
		if err != nil {
			return nil, false, err
		}
		for _, version := range page.Items {
			if version.Deleted {
				continue
			}
			_, sourceMatched := entityIDs[version.Knowledge.SourceEntityID]
			_, targetMatched := entityIDs[version.Knowledge.TargetEntityID]
			if !sourceMatched && !targetMatched {
				continue
			}
			if len(result) == limit {
				return result, true, nil
			}
			reference := knowledge.Reference[knowledge.RelationID]{
				ID:      version.Knowledge.ID,
				Version: version.Version,
			}
			evidence, err := searcher.Evidence.ReadRelationEvidence(ctx, reference)
			if err != nil {
				return nil, false, err
			}
			result = append(result, RelationMatch{
				Version:  version,
				Reason:   "connected_to_entity",
				Evidence: evidence,
			})
		}
		if !page.HasMore {
			return result, false, nil
		}
		after = page.NextAfter
	}
}

func (searcher *Searcher) readClaims(
	ctx context.Context,
	view knowledge.View,
	selected []entitySelection,
	relations []RelationMatch,
	limit int,
) ([]ClaimMatch, bool, error) {
	subjects := make(map[knowledge.ObjectRef]struct{}, len(selected)+len(relations))
	for _, selection := range selected {
		subjects[selection.Reference.ID] = struct{}{}
	}
	for _, relation := range relations {
		subjects[relation.Version.Knowledge.ID] = struct{}{}
	}
	result := make([]ClaimMatch, 0, limit)
	for after := knowledge.ClaimID(""); ; {
		page, err := view.Claims().Current(ctx, after, 256)
		if err != nil {
			return nil, false, err
		}
		for _, version := range page.Items {
			if version.Deleted {
				continue
			}
			if _, matched := subjects[version.Knowledge.Subject]; !matched {
				continue
			}
			if len(result) == limit {
				return result, true, nil
			}
			reference := knowledge.Reference[knowledge.ClaimID]{
				ID:      version.Knowledge.ID,
				Version: version.Version,
			}
			evidence, err := searcher.Evidence.ReadClaimEvidence(ctx, reference)
			if err != nil {
				return nil, false, err
			}
			result = append(result, ClaimMatch{
				Version:  version,
				Reason:   "describes_selected_subject",
				Evidence: evidence,
			})
		}
		if !page.HasMore {
			return result, false, nil
		}
		after = page.NextAfter
	}
}

func normalizeQuery(query Query) (Query, error) {
	query.Title = strings.TrimSpace(query.Title)
	query.Type = strings.TrimSpace(query.Type)
	if query.Title == "" {
		return Query{}, fmt.Errorf("%w: Entity title is required", ErrInvalid)
	}
	limits := []*int{
		&query.Limits.Entities,
		&query.Limits.Relations,
		&query.Limits.Claims,
		&query.Limits.Evidence,
	}
	defaults := []int{
		defaultEntityLimit,
		defaultRelationLimit,
		defaultClaimLimit,
		defaultEvidenceLimit,
	}
	for index, limit := range limits {
		if *limit == 0 {
			*limit = defaults[index]
		}
		if *limit < 1 || *limit > maximumSearchLimit {
			return Query{}, fmt.Errorf("%w: search limits must be between 1 and %d", ErrInvalid, maximumSearchLimit)
		}
	}
	return query, nil
}

func (searcher *Searcher) readEvidence(
	ctx context.Context,
	entities []EntityMatch,
	relations []RelationMatch,
	claims []ClaimMatch,
	limit int,
) ([]Evidence, bool, error) {
	references := collectEvidence(entities, relations, claims)
	truncated := len(references) > limit
	if truncated {
		references = references[:limit]
	}
	result := make([]Evidence, 0, len(references))
	for _, reference := range references {
		switch source := reference.Source.(type) {
		case provenance.MessageSource:
			occurrences, err := searcher.Messages.Read(ctx, []string{source.MessageID})
			if err != nil {
				return nil, false, err
			}
			if len(occurrences) != 1 ||
				occurrences[0].Message.ID != source.MessageID ||
				string(occurrences[0].Message.TextUnit.ID) != reference.TextUnitID {
				return nil, false, errors.New("Message Evidence differs from its Knowledge reference")
			}
			result = append(result, Evidence{
				Reference: reference,
				Source: MessageEvidence{
					Occurrence: occurrences[0],
				},
			})
		case provenance.CorporaSource:
			locations, err := searcher.Corpora.TextUnitLocations(
				ctx,
				corpus.CorporaID(source.CorporaID),
				[]textunits.TextUnitID{textunits.TextUnitID(reference.TextUnitID)},
			)
			if err != nil {
				return nil, false, err
			}
			if len(locations) == 0 {
				return nil, false, errors.New("Corpora Evidence is missing from its source")
			}
			for _, location := range locations {
				if location.CorporaID != corpus.CorporaID(source.CorporaID) ||
					string(location.TextUnit.TextUnit.ID) != reference.TextUnitID {
					return nil, false, errors.New("Corpora Evidence differs from its Knowledge reference")
				}
				result = append(result, Evidence{
					Reference: reference,
					Source: CorporaEvidence{
						Location: location,
					},
				})
			}
		default:
			return nil, false, fmt.Errorf("unsupported Evidence Source %T", reference.Source)
		}
	}
	return result, truncated, nil
}

func collectEvidence(
	entities []EntityMatch,
	relations []RelationMatch,
	claims []ClaimMatch,
) []provenance.Evidence {
	values := make([]provenance.Evidence, 0)
	for _, entity := range entities {
		for _, source := range entity.Evidence {
			values = append(values, source.Metadata.Evidence...)
		}
	}
	for _, relation := range relations {
		for _, source := range relation.Evidence {
			values = append(values, source.Metadata.Evidence...)
		}
	}
	for _, claim := range claims {
		for _, source := range claim.Evidence {
			for _, metadata := range source.Metadata {
				values = append(values, metadata.Evidence...)
			}
		}
	}
	return provenance.CanonicalEvidence(values)
}
