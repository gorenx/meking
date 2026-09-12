// Package adapter maps Query Local consumer contracts onto provider-owned
// Knowledge and completion APIs.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
)

type KnowledgeViewSource interface {
	OpenCurrent(context.Context) (knowledge.View, error)
}

type KnowledgeMetadata interface {
	Entity(context.Context, knowledge.Reference[knowledge.EntityID]) (provenance.EntityMetadata, error)
	Relation(context.Context, knowledge.Reference[knowledge.RelationID]) (provenance.RelationMetadata, error)
	Claim(context.Context, knowledge.Reference[knowledge.ClaimID]) ([]provenance.ClaimMetadata, error)
}

type KnowledgeReader struct {
	source   KnowledgeViewSource
	metadata KnowledgeMetadata
}

var _ querylocal.KnowledgeReader = (*KnowledgeReader)(nil)

func NewKnowledgeReader(source KnowledgeViewSource, metadata KnowledgeMetadata) (*KnowledgeReader, error) {
	if source == nil {
		return nil, errors.New("create Local Knowledge Reader: Knowledge view source is required")
	}
	if metadata == nil {
		return nil, errors.New("create Local Knowledge Reader: Knowledge Metadata is required")
	}
	return &KnowledgeReader{
		source:   source,
		metadata: metadata,
	}, nil
}

func (reader *KnowledgeReader) CurrentEntityReferences(
	ctx context.Context,
	versions knowledge.Manifest,
	entityIDs []string,
) ([]querybase.KnowledgeReference, error) {
	if reader == nil || reader.source == nil || reader.metadata == nil {
		return nil, errors.New("Local Knowledge Reader is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := versions.Validate(); err != nil {
		return nil, err
	}
	available := make(map[string]knowledge.Version, len(versions.Entities))
	for _, reference := range versions.Entities {
		available[string(reference.ID)] = reference.Version
	}
	seen := make(map[string]struct{}, len(entityIDs))
	result := make([]querybase.KnowledgeReference, 0, len(entityIDs))
	for index, id := range entityIDs {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return nil, fmt.Errorf("Local Entity ID %d is invalid", index)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("Local Entity ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
		if version, found := available[id]; found {
			result = append(result, querybase.KnowledgeReference{
				ID:      id,
				Version: uint64(version),
			})
		}
	}
	return result, nil
}

func (reader *KnowledgeReader) Read(
	ctx context.Context,
	versions knowledge.Manifest,
	request querylocal.KnowledgeRequest,
) (_ querylocal.Knowledge, resultErr error) {
	if reader == nil || reader.source == nil || reader.metadata == nil {
		return querylocal.Knowledge{}, errors.New("Local Knowledge Reader is not configured")
	}
	if err := versions.Validate(); err != nil {
		return querylocal.Knowledge{}, err
	}
	view, err := reader.source.OpenCurrent(ctx)
	if err != nil {
		return querylocal.Knowledge{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	allRelations, err := view.Relations().Read(ctx, versions.Relations)
	if err != nil {
		return querylocal.Knowledge{}, fmt.Errorf("read Local graph: %w", err)
	}
	if len(allRelations) != len(versions.Relations) {
		return querylocal.Knowledge{}, errors.New("Local graph is incomplete")
	}
	degrees := make(map[knowledge.EntityID]int)
	relationsByReference := make(map[knowledge.Reference[knowledge.RelationID]]knowledge.KnowledgeVersion[knowledge.Relation], len(allRelations))
	for index, version := range allRelations {
		reference := versions.Relations[index]
		if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
			return querylocal.Knowledge{}, errors.New("Local graph differs from its exact Version set")
		}
		degrees[version.Knowledge.SourceEntityID]++
		degrees[version.Knowledge.TargetEntityID]++
		relationsByReference[reference] = version
	}
	result := querylocal.Knowledge{
		Entities:      make([]querylocal.Entity, len(request.Entities)),
		Relationships: make([]querylocal.Relationship, len(request.Relations)),
		Claims:        make([]querylocal.Claim, len(request.Claims)),
	}
	entityReferences, err := entityReferences(request.Entities, versions.Entities)
	if err != nil {
		return querylocal.Knowledge{}, err
	}
	entities, err := view.Entities().Read(ctx, entityReferences)
	if err != nil {
		return querylocal.Knowledge{}, fmt.Errorf("read Local Entity Versions: %w", err)
	}
	for index, version := range entities {
		if err := requireExact(version.Deleted, string(version.Knowledge.ID), version.Version, request.Entities[index]); err != nil {
			return querylocal.Knowledge{}, err
		}
		metadata, err := reader.metadata.Entity(ctx, entityReferences[index])
		if err != nil {
			return querylocal.Knowledge{}, err
		}
		result.Entities[index] = querylocal.Entity{
			ID:          string(version.Knowledge.ID),
			Version:     uint64(version.Version),
			Title:       version.Knowledge.Title,
			Description: version.Knowledge.Description,
			TextUnitIDs: evidenceTextUnitIDs(metadata.Evidence),
			Degree:      degrees[version.Knowledge.ID],
		}
	}
	relationReferences, err := relationReferences(request.Relations, versions.Relations)
	if err != nil {
		return querylocal.Knowledge{}, err
	}
	for index, reference := range relationReferences {
		version, found := relationsByReference[reference]
		if !found {
			return querylocal.Knowledge{}, errors.New("Local Relation Version is missing")
		}
		if err := requireExact(version.Deleted, string(version.Knowledge.ID), version.Version, request.Relations[index]); err != nil {
			return querylocal.Knowledge{}, err
		}
		metadata, err := reader.metadata.Relation(ctx, reference)
		if err != nil {
			return querylocal.Knowledge{}, err
		}
		result.Relationships[index] = querylocal.Relationship{
			ID:             string(version.Knowledge.ID),
			Version:        uint64(version.Version),
			SourceEntityID: string(version.Knowledge.SourceEntityID),
			TargetEntityID: string(version.Knowledge.TargetEntityID),
			Description:    version.Knowledge.Description,
			Weight:         metadata.Weight,
			CombinedDegree: degrees[version.Knowledge.SourceEntityID] + degrees[version.Knowledge.TargetEntityID],
			TextUnitIDs:    evidenceTextUnitIDs(metadata.Evidence),
		}
	}
	claimReferences, err := claimReferences(request.Claims, versions.Claims)
	if err != nil {
		return querylocal.Knowledge{}, err
	}
	claims, err := view.Claims().Read(ctx, claimReferences)
	if err != nil {
		return querylocal.Knowledge{}, fmt.Errorf("read Local Claim Versions: %w", err)
	}
	for index, version := range claims {
		reference := request.Claims[index]
		if err := requireExact(
			version.Deleted,
			string(version.Knowledge.ID),
			version.Version,
			querybase.KnowledgeReference{ID: reference.ID, Version: reference.Version},
		); err != nil {
			return querylocal.Knowledge{}, err
		}
		subject, err := claimSubject(version.Knowledge.Subject)
		if err != nil {
			return querylocal.Knowledge{}, err
		}
		metadata, err := reader.metadata.Claim(ctx, claimReferences[index])
		if err != nil {
			return querylocal.Knowledge{}, err
		}
		source, evidence, found := claimEvidence(metadata, reference.EvidenceIndex)
		if !found {
			return querylocal.Knowledge{}, fmt.Errorf("Claim %q Evidence index %d does not exist", reference.ID, reference.EvidenceIndex)
		}
		result.Claims[index] = querylocal.Claim{
			ID:            string(version.Knowledge.ID),
			Version:       uint64(version.Version),
			EvidenceIndex: reference.EvidenceIndex,
			Subject:       subject,
			Type:          version.Knowledge.Type,
			Status:        source.Status,
			StartDate:     source.StartDate,
			EndDate:       source.EndDate,
			Description:   version.Knowledge.Description,
			SourceText:    source.SourceText,
			TextUnitID:    evidence.TextUnitID,
		}
	}
	return result, nil
}

func claimEvidence(
	metadata []provenance.ClaimMetadata,
	index int,
) (provenance.ClaimMetadata, provenance.Evidence, bool) {
	if index < 0 {
		return provenance.ClaimMetadata{}, provenance.Evidence{}, false
	}
	current := 0
	for _, source := range metadata {
		for _, evidence := range source.Evidence {
			if current == index {
				return source, evidence, true
			}
			current++
		}
	}
	return provenance.ClaimMetadata{}, provenance.Evidence{}, false
}

func evidenceTextUnitIDs(evidence []provenance.Evidence) []string {
	unique := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		unique[item.TextUnitID] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for textUnitID := range unique {
		result = append(result, textUnitID)
	}
	slices.Sort(result)
	return result
}

func entityReferences(
	requested []querybase.KnowledgeReference,
	available []knowledge.Reference[knowledge.EntityID],
) ([]knowledge.Reference[knowledge.EntityID], error) {
	allowed := make(map[knowledge.Reference[knowledge.EntityID]]struct{}, len(available))
	for _, reference := range available {
		allowed[reference] = struct{}{}
	}
	result := make([]knowledge.Reference[knowledge.EntityID], len(requested))
	for index, reference := range requested {
		result[index] = knowledge.Reference[knowledge.EntityID]{
			ID:      knowledge.EntityID(reference.ID),
			Version: knowledge.Version(reference.Version),
		}
		if _, found := allowed[result[index]]; !found {
			return nil, errors.New("Local Entity reference is outside the Epoch Knowledge set")
		}
	}
	return result, nil
}

func relationReferences(
	requested []querybase.KnowledgeReference,
	available []knowledge.Reference[knowledge.RelationID],
) ([]knowledge.Reference[knowledge.RelationID], error) {
	allowed := make(map[knowledge.Reference[knowledge.RelationID]]struct{}, len(available))
	for _, reference := range available {
		allowed[reference] = struct{}{}
	}
	result := make([]knowledge.Reference[knowledge.RelationID], len(requested))
	for index, reference := range requested {
		result[index] = knowledge.Reference[knowledge.RelationID]{
			ID:      knowledge.RelationID(reference.ID),
			Version: knowledge.Version(reference.Version),
		}
		if _, found := allowed[result[index]]; !found {
			return nil, errors.New("Local Relation reference is outside the Epoch Knowledge set")
		}
	}
	return result, nil
}

func claimReferences(
	requested []querybase.ClaimReference,
	available []knowledge.Reference[knowledge.ClaimID],
) ([]knowledge.Reference[knowledge.ClaimID], error) {
	allowed := make(map[knowledge.Reference[knowledge.ClaimID]]struct{}, len(available))
	for _, reference := range available {
		allowed[reference] = struct{}{}
	}
	result := make([]knowledge.Reference[knowledge.ClaimID], len(requested))
	for index, reference := range requested {
		result[index] = knowledge.Reference[knowledge.ClaimID]{
			ID:      knowledge.ClaimID(reference.ID),
			Version: knowledge.Version(reference.Version),
		}
		if _, found := allowed[result[index]]; !found {
			return nil, errors.New("Local Claim reference is outside the Epoch Knowledge set")
		}
	}
	return result, nil
}

func requireExact(
	deleted bool,
	id string,
	version knowledge.Version,
	reference querybase.KnowledgeReference,
) error {
	if deleted || id != reference.ID || uint64(version) != reference.Version {
		return errors.New("Local Knowledge differs from its exact Version reference")
	}
	return nil
}

func claimSubject(subject knowledge.Subject) (querylocal.ClaimSubject, error) {
	switch subject := subject.(type) {
	case knowledge.EntityID:
		return querylocal.EntityClaimSubject{ID: string(subject)}, nil
	case knowledge.RelationID:
		return querylocal.RelationClaimSubject{ID: string(subject)}, nil
	default:
		return nil, fmt.Errorf("map Local Claim Subject %T: unsupported Knowledge Subject", subject)
	}
}
