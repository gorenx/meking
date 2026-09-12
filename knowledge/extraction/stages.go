package extraction

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/zone"
)

func (service *Service) ExtractTextUnit(
	ctx context.Context,
	corporaID string,
	unit TextUnitInput,
) (Result, error) {
	graph, err := service.extractGraph(ctx, corporaID, []TextUnitInput{unit})
	if err != nil {
		return Result{}, err
	}
	claims, err := service.extractClaims(ctx, corporaID, []TextUnitInput{unit}, graph)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Entities:  graph.Entities,
		Relations: graph.Relations,
		Claims:    claims,
	}, nil
}

func (service *Service) extractGraph(
	ctx context.Context,
	corporaID string,
	textUnits []TextUnitInput,
) (Graph, error) {
	if service == nil {
		return Graph{}, errors.New("Extraction Service is not configured")
	}
	if err := validateTextUnits(textUnits); err != nil {
		return Graph{}, err
	}
	config := service.policy.Graph
	config.MaxConcurrency = service.model.maxConcurrency
	extractor, err := newGraphExtractor(service.model.completion, config)
	if err != nil {
		return Graph{}, err
	}
	observations, err := extractor.extractGraph(ctx, textUnits)
	if err != nil {
		return Graph{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return Graph{}, err
	}
	return graphFromObservations(string(zoneID), corporaID, observations)
}

func (service *Service) extractClaims(
	ctx context.Context,
	corporaID string,
	textUnits []TextUnitInput,
	graph Graph,
) ([]submission.Claim, error) {
	if err := validateTextUnits(textUnits); err != nil {
		return nil, err
	}
	if service.policy.Claims == nil {
		return []submission.Claim{}, nil
	}
	config := *service.policy.Claims
	config.MaxConcurrency = service.model.maxConcurrency
	extractor, err := newclaimExtractor(service.model.completion, config)
	if err != nil {
		return nil, err
	}
	claims, err := extractor.extractClaims(ctx, textUnits)
	if err != nil {
		return nil, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, err
	}
	identities := identitiesByTitle(graph.Entities)
	result := make([]submission.Claim, 0, len(claims))
	for _, claim := range claims {
		subject, err := resolveEntityIdentity(claim.Subject, identities)
		if err != nil {
			return nil, fmt.Errorf("resolve extracted Claim Subject: %w", err)
		}
		result = append(result, submission.Claim{
			Content: knowledge.ClaimContent{
				Subject:     subject,
				Type:        claim.Type,
				Description: claim.Description,
			},
			Metadata: provenance.ClaimMetadata{
				SubjectText: claim.Subject,
				ObjectText:  claim.Object,
				Status:      claim.Status,
				StartDate:   claim.StartDate,
				EndDate:     claim.EndDate,
				SourceText:  claim.SourceText,
				Evidence: []provenance.Evidence{
					{
						ZoneID:     string(zoneID),
						TextUnitID: claim.TextUnitID,
						Source:     provenance.CorporaSource{CorporaID: corporaID},
					},
				},
			},
		})
	}
	return result, nil
}

func graphFromObservations(zoneID string, corporaID string, observations graphObservations) (Graph, error) {
	if len(observations.Entities) == 0 {
		return Graph{}, ErrNoEntitiesDetected
	}
	identities := make(map[string][]knowledge.EntityIdentity, len(observations.Entities))
	result := Graph{
		Entities:  make([]submission.Entity, 0, len(observations.Entities)),
		Relations: make([]submission.Relation, 0, len(observations.Relationships)),
	}
	for _, observation := range observations.Entities {
		identity, err := knowledge.NewEntityIdentity(observation.Title, observation.Type)
		if err != nil {
			return Graph{}, fmt.Errorf("build extracted Entity identity: %w", err)
		}
		identities[identity.Title] = appendIdentity(identities[identity.Title], identity)
		result.Entities = append(result.Entities, submission.Entity{
			Content: knowledge.EntityContent{
				Identity:    identity,
				Aliases:     append([]string(nil), observation.Aliases...),
				Description: observation.Description,
			},
			Metadata: provenance.EntityMetadata{
				Frequency: 1,
				Evidence: []provenance.Evidence{
					{
						ZoneID:     zoneID,
						TextUnitID: observation.TextUnitID,
						Source:     provenance.CorporaSource{CorporaID: corporaID},
					},
				},
			},
		})
	}
	for _, observation := range observations.Relationships {
		source, err := resolveEntityIdentity(observation.Source, identities)
		if err != nil {
			return Graph{}, fmt.Errorf("resolve extracted Relation source: %w", err)
		}
		target, err := resolveEntityIdentity(observation.Target, identities)
		if err != nil {
			return Graph{}, fmt.Errorf("resolve extracted Relation target: %w", err)
		}
		result.Relations = append(result.Relations, submission.Relation{
			Content: knowledge.RelationContent{
				Source:      source,
				Target:      target,
				Type:        observation.Type,
				Description: observation.Description,
			},
			Metadata: provenance.RelationMetadata{
				Weight: observation.Weight,
				Evidence: []provenance.Evidence{
					{
						ZoneID:     zoneID,
						TextUnitID: observation.TextUnitID,
						Source:     provenance.CorporaSource{CorporaID: corporaID},
					},
				},
			},
		})
	}
	return result, nil
}

func identitiesByTitle(entities []submission.Entity) map[string][]knowledge.EntityIdentity {
	result := make(map[string][]knowledge.EntityIdentity, len(entities))
	for _, entity := range entities {
		identity := entity.Content.Identity
		result[identity.Title] = appendIdentity(result[identity.Title], identity)
	}
	return result
}

func appendIdentity(
	identities []knowledge.EntityIdentity,
	identity knowledge.EntityIdentity,
) []knowledge.EntityIdentity {
	for _, existing := range identities {
		if existing == identity {
			return identities
		}
	}
	return append(identities, identity)
}

func resolveEntityIdentity(
	title string,
	identities map[string][]knowledge.EntityIdentity,
) (knowledge.EntityIdentity, error) {
	matches := identities[cleanGraphField(upperGraphField(title))]
	switch len(matches) {
	case 0:
		return knowledge.EntityIdentity{}, fmt.Errorf("Entity title %q is unresolved", title)
	case 1:
		return matches[0], nil
	default:
		return knowledge.EntityIdentity{}, fmt.Errorf("Entity title %q has multiple Types", title)
	}
}
