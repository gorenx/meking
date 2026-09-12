package report

import (
	"fmt"
	"sort"

	"github.com/memoria-space/meking/community"
)

// reportContext is the complete, validated model request for one Community
// together with the exact source identities that the resulting Report records.
// It exists only between input validation and model completion.
type reportContext struct {
	community   community.Membership
	prompt      string
	fragments   []string
	entities    []community.EntityReference
	relations   []community.RelationReference
	claims      []ClaimSource
	textUnitIDs []string
}

// communityInput is the complete set of validated rows that one Community
// contributes to a report request before CSV rendering. It centralizes the
// membership, internal-Relation, Claim-subject, and ordering rules used by both
// generation and input-difference decisions.
type communityInput struct {
	entities  []Entity
	relations []Relation
	claims    []Claim
}

func buildReportContexts(
	input Input,
	tokens community.ReportTokenCounter,
	config Config,
) ([]reportContext, error) {
	entitiesByID, relationsByID, err := validateReportInput(input)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	communities := append(
		[]community.Membership(nil),
		input.Communities...,
	)
	sort.Slice(communities, func(left, right int) bool {
		if communities[left].Level != communities[right].Level {
			return communities[left].Level > communities[right].Level
		}
		return communities[left].Number < communities[right].Number
	})

	result := make([]reportContext, len(communities))
	for index, current := range communities {
		context, err := buildReportContext(
			current,
			input.Communities,
			entitiesByID,
			relationsByID,
			input.Relations,
			input.Claims,
			tokens,
			config,
		)
		if err != nil {
			return nil, err
		}
		result[index] = context
	}
	return result, nil
}

func buildReportContext(
	current community.Membership,
	communities []community.Membership,
	entitiesByID map[string]Entity,
	relationsByID map[string]Relation,
	relations []Relation,
	claims []Claim,
	tokens community.ReportTokenCounter,
	config Config,
) (reportContext, error) {
	input := selectCommunityInput(
		current,
		entitiesByID,
		relationsByID,
		relations,
		claims,
	)
	text, err := renderCompleteReportContext(
		input.entities,
		input.claims,
		input.relations,
		entitiesByID,
	)
	if err != nil {
		return reportContext{}, fmt.Errorf(
			"render Report context for Community %q: %w",
			current.ID,
			err,
		)
	}
	prompt, err := community.RenderReportPrompt(config.Prompt, text, config.MaxReportLength)
	if err != nil {
		return reportContext{}, fmt.Errorf(
			"render Report prompt for Community %q: %w",
			current.ID,
			err,
		)
	}
	count, err := tokens.Count(prompt)
	if err != nil {
		return reportContext{}, fmt.Errorf(
			"count Report prompt for Community %q: %w",
			current.ID,
			err,
		)
	}
	var fragmentPrompts []string
	if count > config.MaxInputTokens {
		fragmentPrompts, err = planReportFragments(
			current,
			communities,
			input,
			entitiesByID,
			tokens,
			config,
		)
		if err != nil {
			return reportContext{}, err
		}
		prompt = ""
	}

	entitySources := make([]community.EntityReference, len(input.entities))
	relationSources := make([]community.RelationReference, len(input.relations))
	claimSources := make([]ClaimSource, len(input.claims))
	evidence := make(map[string]struct{})
	for index, entity := range input.entities {
		entitySources[index] = community.EntityReference{
			ID:      entity.ID,
			Version: entity.Version,
		}
		addReportEvidence(evidence, entity.TextUnitIDs)
	}
	for index, relation := range input.relations {
		relationSources[index] = community.RelationReference{
			ID:      relation.ID,
			Version: relation.Version,
		}
		addReportEvidence(evidence, relation.TextUnitIDs)
	}
	for index, claim := range input.claims {
		claimSources[index] = ClaimSource{
			ID:            claim.ID,
			Version:       claim.Version,
			EvidenceIndex: claim.EvidenceIndex,
		}
		evidence[claim.TextUnitID] = struct{}{}
	}
	textUnitIDs := make([]string, 0, len(evidence))
	for textUnitID := range evidence {
		textUnitIDs = append(textUnitIDs, textUnitID)
	}
	sort.Strings(textUnitIDs)

	return reportContext{
		community:   current,
		prompt:      prompt,
		fragments:   fragmentPrompts,
		entities:    entitySources,
		relations:   relationSources,
		claims:      claimSources,
		textUnitIDs: textUnitIDs,
	}, nil
}

func selectCommunityInput(
	current community.Membership,
	entitiesByID map[string]Entity,
	relationsByID map[string]Relation,
	relations []Relation,
	claims []Claim,
) communityInput {
	members := make(map[string]struct{}, len(current.EntityIDs))
	entities := make([]Entity, len(current.EntityIDs))
	for index, entityID := range current.EntityIDs {
		members[entityID] = struct{}{}
		entities[index] = entitiesByID[entityID]
	}
	sort.Slice(entities, func(left, right int) bool {
		if entities[left].Degree != entities[right].Degree {
			return entities[left].Degree > entities[right].Degree
		}
		return entities[left].ID < entities[right].ID
	})

	selectedRelations := make([]Relation, 0)
	for _, relation := range relations {
		if _, found := members[relation.SourceEntityID]; !found {
			continue
		}
		if _, found := members[relation.TargetEntityID]; !found {
			continue
		}
		selectedRelations = append(selectedRelations, relation)
	}
	sort.Slice(selectedRelations, func(left, right int) bool {
		if selectedRelations[left].CombinedDegree !=
			selectedRelations[right].CombinedDegree {
			return selectedRelations[left].CombinedDegree >
				selectedRelations[right].CombinedDegree
		}
		return selectedRelations[left].ID < selectedRelations[right].ID
	})

	selectedClaims := make([]Claim, 0)
	for _, claim := range claims {
		if reportClaimBelongsToCommunity(claim, members, relationsByID) {
			selectedClaims = append(selectedClaims, claim)
		}
	}
	return communityInput{
		entities:  entities,
		relations: selectedRelations,
		claims:    selectedClaims,
	}
}

func renderCompleteReportContext(
	entities []Entity,
	claims []Claim,
	relations []Relation,
	entitiesByID map[string]Entity,
) (string, error) {
	return renderReportBatch(numberReportInput(communityInput{
		entities: entities, relations: relations, claims: claims,
	}), entitiesByID)
}

func addReportEvidence(result map[string]struct{}, ids []string) {
	for _, id := range ids {
		result[id] = struct{}{}
	}
}

func reportClaimBelongsToCommunity(
	claim Claim,
	members map[string]struct{},
	relations map[string]Relation,
) bool {
	switch claim.Subject.Kind {
	case EntityClaimSubject:
		_, found := members[claim.Subject.ID]
		return found
	case RelationClaimSubject:
		relation, found := relations[claim.Subject.ID]
		if !found {
			return false
		}
		_, sourceFound := members[relation.SourceEntityID]
		_, targetFound := members[relation.TargetEntityID]
		return sourceFound && targetFound
	default:
		return false
	}
}
