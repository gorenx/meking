package knowledge

import (
	"context"
	"errors"
	"fmt"
	"slices"

	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/community/reportgeneration"
	knowledgebase "github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

func (reader *Reader) ReportSnapshot(
	ctx context.Context,
	versions knowledgebase.Manifest,
) (_ reportgeneration.KnowledgeSnapshot, resultErr error) {
	if reader == nil || reader.source == nil {
		return reportgeneration.KnowledgeSnapshot{}, errors.New(
			"read Community Report Knowledge snapshot: Reader is not initialized",
		)
	}
	if err := versions.Validate(); err != nil {
		return reportgeneration.KnowledgeSnapshot{}, err
	}
	view, err := reader.source.OpenCurrent(ctx)
	if err != nil {
		return reportgeneration.KnowledgeSnapshot{}, fmt.Errorf("open Community Report Knowledge: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	entities, err := view.Entities().Read(ctx, versions.Entities)
	if err != nil {
		return reportgeneration.KnowledgeSnapshot{}, err
	}
	relations, err := view.Relations().Read(ctx, versions.Relations)
	if err != nil {
		return reportgeneration.KnowledgeSnapshot{}, err
	}
	claims, err := view.Claims().Read(ctx, versions.Claims)
	if err != nil {
		return reportgeneration.KnowledgeSnapshot{}, err
	}
	if len(entities) != len(versions.Entities) ||
		len(relations) != len(versions.Relations) ||
		len(claims) != len(versions.Claims) {
		return reportgeneration.KnowledgeSnapshot{}, errors.New("Community Report Knowledge snapshot is incomplete")
	}
	result := reportgeneration.KnowledgeSnapshot{
		Entities:  make([]communityreport.Entity, len(entities)),
		Relations: make([]communityreport.Relation, len(relations)),
		Claims:    []communityreport.Claim{},
	}
	degrees := make(map[knowledgebase.EntityID]int, len(entities))
	for _, version := range relations {
		degrees[version.Knowledge.SourceEntityID]++
		degrees[version.Knowledge.TargetEntityID]++
	}
	for index, version := range entities {
		reference := versions.Entities[index]
		if err := requireEntityVersion(version, reference); err != nil {
			return reportgeneration.KnowledgeSnapshot{}, err
		}
		metadata, err := reader.metadata.Entity(ctx, reference)
		if err != nil {
			return reportgeneration.KnowledgeSnapshot{}, err
		}
		result.Entities[index] = communityreport.Entity{
			ID:          string(version.Knowledge.ID),
			Version:     uint64(version.Version),
			Title:       version.Knowledge.Title,
			Description: version.Knowledge.Description,
			Degree:      degrees[version.Knowledge.ID],
			TextUnitIDs: evidenceIDs(metadata.Evidence),
		}
	}
	for index, version := range relations {
		reference := versions.Relations[index]
		if err := requireRelationVersion(version, reference); err != nil {
			return reportgeneration.KnowledgeSnapshot{}, err
		}
		metadata, err := reader.metadata.Relation(ctx, reference)
		if err != nil {
			return reportgeneration.KnowledgeSnapshot{}, err
		}
		relation := version.Knowledge
		result.Relations[index] = communityreport.Relation{
			ID:             string(relation.ID),
			Version:        uint64(version.Version),
			SourceEntityID: string(relation.SourceEntityID),
			TargetEntityID: string(relation.TargetEntityID),
			Description:    relation.Description,
			Weight:         metadata.Weight,
			CombinedDegree: degrees[relation.SourceEntityID] + degrees[relation.TargetEntityID],
			TextUnitIDs:    evidenceIDs(metadata.Evidence),
		}
	}
	for index, version := range claims {
		reference := versions.Claims[index]
		if err := requireClaimVersion(version, reference); err != nil {
			return reportgeneration.KnowledgeSnapshot{}, err
		}
		subject, err := projectClaimSubject(version.Knowledge.Subject)
		if err != nil {
			return reportgeneration.KnowledgeSnapshot{}, err
		}
		metadata, err := reader.metadata.Claim(ctx, reference)
		if err != nil {
			return reportgeneration.KnowledgeSnapshot{}, err
		}
		claimIndex := 0
		for _, source := range metadata {
			for _, evidence := range source.Evidence {
				subjectText := source.SubjectText
				if subjectText == "" {
					subjectText = subject.ID
				}
				result.Claims = append(result.Claims, communityreport.Claim{
					ID:            string(version.Knowledge.ID),
					Version:       uint64(version.Version),
					EvidenceIndex: claimIndex,
					Subject:       subject,
					SubjectText:   subjectText,
					ObjectText:    source.ObjectText,
					Type:          version.Knowledge.Type,
					Status:        source.Status,
					StartDate:     source.StartDate,
					EndDate:       source.EndDate,
					Description:   version.Knowledge.Description,
					SourceText:    source.SourceText,
					TextUnitID:    evidence.TextUnitID,
				})
				claimIndex++
			}
		}
	}
	return result, nil
}

func evidenceIDs(evidence []provenance.Evidence) []string {
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

func requireEntityVersion(
	version knowledgebase.KnowledgeVersion[knowledgebase.Entity],
	reference knowledgebase.Reference[knowledgebase.EntityID],
) error {
	if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
		return errors.New("Community Report Entity differs from its exact Version reference")
	}
	return nil
}

func requireRelationVersion(
	version knowledgebase.KnowledgeVersion[knowledgebase.Relation],
	reference knowledgebase.Reference[knowledgebase.RelationID],
) error {
	if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
		return errors.New("Community Report Relation differs from its exact Version reference")
	}
	return nil
}

func requireClaimVersion(
	version knowledgebase.KnowledgeVersion[knowledgebase.Claim],
	reference knowledgebase.Reference[knowledgebase.ClaimID],
) error {
	if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
		return errors.New("Community Report Claim differs from its exact Version reference")
	}
	return nil
}

func projectClaimSubject(subject knowledgebase.Subject) (communityreport.ClaimSubject, error) {
	switch value := subject.(type) {
	case knowledgebase.EntityID:
		return communityreport.ClaimSubject{
			Kind: communityreport.EntityClaimSubject,
			ID:   string(value),
		}, nil
	case knowledgebase.RelationID:
		return communityreport.ClaimSubject{
			Kind: communityreport.RelationClaimSubject,
			ID:   string(value),
		}, nil
	default:
		return communityreport.ClaimSubject{}, fmt.Errorf("Knowledge Subject %T is invalid", subject)
	}
}
