package local

import (
	"context"
	"fmt"
	"sort"

	"github.com/memoria-space/meking/knowledge"
	querybase "github.com/memoria-space/meking/query"
	queryreport "github.com/memoria-space/meking/query/report"
)

// rankedReport binds one immutable Report to its Community membership and the
// request-local match count used to order candidate Report rows.
type rankedReport struct {
	report     queryreport.PublishedReport
	community  queryreport.PublishedCommunity
	matchCount int
}

// localEvidence keeps the exact rows selected from the independently fixed
// Report and Entity-vector views before token budgeting decides which prefix of
// each table becomes model-visible.
type localEvidence struct {
	selectedEntities []Entity
	reportEntities   []Entity
	reports          []rankedReport
	relationships    []Relationship
	claims           []Claim
}

func (s *Searcher) selectEntityMatches(
	ctx context.Context,
	versions knowledge.Manifest,
	vectors EntityVectorReader,
	queryVector []float64,
	request SearchRequest,
) ([]EntityMatch, error) {
	excluded, _ := validateEntityIDs(request.ExcludeEntityIDs, "excluded")
	result := make([]EntityMatch, 0, len(request.IncludeEntityIDs)+s.config.TopKEntities)
	seen := make(map[string]uint64, cap(result))
	if len(request.IncludeEntityIDs) > 0 {
		references, err := s.knowledge.CurrentEntityReferences(
			ctx, versions, append([]string(nil), request.IncludeEntityIDs...),
		)
		if err != nil {
			return nil, normalizeFailure(err)
		}
		if len(references) != len(request.IncludeEntityIDs) {
			return nil, querybase.NewInvalidInputFailure(
				"one or more explicitly included Entities are absent from the fixed Knowledge view",
				nil,
			)
		}
		for index, reference := range references {
			match := EntityMatch{ID: reference.ID, Version: reference.Version, Score: 1}
			if err := validateEntityMatch(match); err != nil {
				return nil, querybase.NewPublicationIncompleteFailure(err)
			}
			if match.ID != request.IncludeEntityIDs[index] {
				return nil, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
					"Local Entity Get returned %q for requested %q", match.ID, request.IncludeEntityIDs[index],
				))
			}
			seen[match.ID] = match.Version
			result = append(result, match)
		}
	}
	candidates, err := vectors.Search(ctx, queryVector, s.config.TopKEntities*2)
	if err != nil {
		return nil, normalizeFailure(err)
	}
	semanticCount := 0
	for _, match := range candidates {
		if err := validateEntityMatch(match); err != nil {
			return nil, querybase.NewPublicationIncompleteFailure(err)
		}
		if _, omitted := excluded[match.ID]; omitted {
			continue
		}
		if version, duplicate := seen[match.ID]; duplicate {
			if version != match.Version {
				return nil, querybase.NewPublicationIncompleteFailure(fmt.Errorf(
					"Local Entity vector view returned versions %d and %d for %q",
					version, match.Version, match.ID,
				))
			}
			continue
		}
		seen[match.ID] = match.Version
		result = append(result, match)
		semanticCount++
		if semanticCount == s.config.TopKEntities {
			break
		}
	}
	return result, nil
}

func (s *Searcher) readEvidence(
	ctx context.Context,
	versions knowledge.Manifest,
	view queryreport.View,
	matches []EntityMatch,
) (localEvidence, error) {
	reports := selectReports(view, matches)
	request := knowledgeRequest(reports, matches)
	values, err := s.knowledge.Read(ctx, versions, request)
	if err != nil {
		return localEvidence{}, normalizePublicationFailure(err)
	}
	selected := make([]Entity, len(matches))
	entityByReference := make(map[string]Entity, len(values.Entities))
	for _, entity := range values.Entities {
		entityByReference[referenceKey(entity.ID, entity.Version)] = entity
	}
	for index, match := range matches {
		selected[index] = entityByReference[referenceKey(match.ID, match.Version)]
	}
	reportEntities := make([]Entity, 0, len(request.Entities))
	selectedRefs := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		selectedRefs[referenceKey(match.ID, match.Version)] = struct{}{}
	}
	for _, reference := range request.Entities {
		if _, selectedRef := selectedRefs[referenceKey(reference.ID, reference.Version)]; selectedRef {
			continue
		}
		reportEntities = append(reportEntities, entityByReference[referenceKey(reference.ID, reference.Version)])
	}
	relationships := selectRelationships(values.Relationships, matches, s.config.TopKRelationships)
	acceptedRelationships := make(map[string]struct{}, len(relationships))
	for _, relationship := range relationships {
		acceptedRelationships[relationship.ID] = struct{}{}
	}
	selectedIDs := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		selectedIDs[match.ID] = struct{}{}
	}
	claims := make([]Claim, 0, len(values.Claims))
	for _, claim := range values.Claims {
		include := false
		switch subject := claim.Subject.(type) {
		case EntityClaimSubject:
			_, include = selectedIDs[subject.ID]
		case RelationClaimSubject:
			_, include = acceptedRelationships[subject.ID]
		}
		if include {
			claims = append(claims, claim)
		}
	}
	return localEvidence{
		selectedEntities: selected, reportEntities: reportEntities, reports: reports,
		relationships: relationships, claims: claims,
	}, nil
}

func selectReports(view queryreport.View, matches []EntityMatch) []rankedReport {
	selected := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		selected[match.ID] = struct{}{}
	}
	result := make([]rankedReport, 0)
	for index, community := range view.Communities {
		count := 0
		for _, entityID := range community.EntityIDs {
			if _, matched := selected[entityID]; matched {
				count++
			}
		}
		if count == 0 {
			continue
		}
		result = append(result, rankedReport{
			report: view.Reports[index], community: community, matchCount: count,
		})
	}
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].matchCount != result[right].matchCount {
			return result[left].matchCount > result[right].matchCount
		}
		if result[left].report.Rank != result[right].report.Rank {
			return result[left].report.Rank > result[right].report.Rank
		}
		if result[left].community.Number != result[right].community.Number {
			return result[left].community.Number < result[right].community.Number
		}
		return result[left].report.ID < result[right].report.ID
	})
	return result
}

func knowledgeRequest(
	reports []rankedReport,
	matches []EntityMatch,
) KnowledgeRequest {
	request := KnowledgeRequest{}
	entityRefs := make(map[string]querybase.KnowledgeReference)
	relationRefs := make(map[string]querybase.KnowledgeReference)
	claimRefs := make(map[string]querybase.ClaimReference)
	addEntity := func(reference querybase.KnowledgeReference) {
		key := referenceKey(reference.ID, reference.Version)
		if _, exists := entityRefs[key]; !exists {
			entityRefs[key] = reference
			request.Entities = append(request.Entities, reference)
		}
	}
	for _, match := range matches {
		addEntity(querybase.KnowledgeReference{ID: match.ID, Version: match.Version})
	}
	for _, current := range reports {
		for _, reference := range current.report.Sources.Entities {
			addEntity(reference)
		}
		for _, reference := range current.report.Sources.Relations {
			key := referenceKey(reference.ID, reference.Version)
			if _, exists := relationRefs[key]; !exists {
				relationRefs[key] = reference
				request.Relations = append(request.Relations, reference)
			}
		}
		for _, reference := range current.report.Sources.Claims {
			key := claimReferenceKey(reference)
			if _, exists := claimRefs[key]; !exists {
				claimRefs[key] = reference
				request.Claims = append(request.Claims, reference)
			}
		}
	}
	return request
}

func selectRelationships(
	candidates []Relationship,
	matches []EntityMatch,
	topK int,
) []Relationship {
	selected := make(map[string]int, len(matches))
	for index, match := range matches {
		selected[match.ID] = index
	}
	internal := make([]Relationship, 0)
	external := make([]Relationship, 0)
	externalLinks := make(map[string]map[string]struct{})
	for _, relationship := range candidates {
		_, sourceSelected := selected[relationship.SourceEntityID]
		_, targetSelected := selected[relationship.TargetEntityID]
		switch {
		case sourceSelected && targetSelected:
			internal = append(internal, relationship)
		case sourceSelected != targetSelected:
			external = append(external, relationship)
			other := relationship.SourceEntityID
			selectedID := relationship.TargetEntityID
			if sourceSelected {
				other = relationship.TargetEntityID
				selectedID = relationship.SourceEntityID
			}
			if externalLinks[other] == nil {
				externalLinks[other] = make(map[string]struct{})
			}
			externalLinks[other][selectedID] = struct{}{}
		}
	}
	byImportance := func(values []Relationship, links bool) {
		sort.SliceStable(values, func(left, right int) bool {
			if links {
				leftOther := externalEntity(values[left], selected)
				rightOther := externalEntity(values[right], selected)
				if len(externalLinks[leftOther]) != len(externalLinks[rightOther]) {
					return len(externalLinks[leftOther]) > len(externalLinks[rightOther])
				}
			}
			if values[left].CombinedDegree != values[right].CombinedDegree {
				return values[left].CombinedDegree > values[right].CombinedDegree
			}
			if values[left].Weight != values[right].Weight {
				return values[left].Weight > values[right].Weight
			}
			return values[left].ID < values[right].ID
		})
	}
	byImportance(internal, false)
	byImportance(external, true)
	externalLimit := topK * len(matches)
	if len(external) > externalLimit {
		external = external[:externalLimit]
	}
	return append(internal, external...)
}

func externalEntity(relationship Relationship, selected map[string]int) string {
	if _, exists := selected[relationship.SourceEntityID]; exists {
		return relationship.TargetEntityID
	}
	return relationship.SourceEntityID
}

func referenceKey(id string, version uint64) string {
	return fmt.Sprintf("%s\x00%d", id, version)
}

func claimReferenceKey(reference querybase.ClaimReference) string {
	return fmt.Sprintf("%s\x00%d\x00%d", reference.ID, reference.Version, reference.EvidenceIndex)
}
