package submission

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

type sourceBatch struct {
	source    provenance.Source
	evidence  []provenance.Evidence
	entities  []entityBatch
	relations []relationBatch
	claims    []claimBatch
}

type entityBatch struct {
	key      string
	identity knowledge.EntityIdentity
	variants []entityVariant
}

type entityVariant struct {
	key         string
	aliases     []string
	description string
	metadata    provenance.EntityMetadata
}

type relationBatch struct {
	key      string
	source   knowledge.EntityIdentity
	target   knowledge.EntityIdentity
	relType  string
	variants []relationVariant
}

type relationVariant struct {
	key         string
	description string
	metadata    provenance.RelationMetadata
}

type claimBatch struct {
	key        string
	subjectKey string
	subject    knowledge.SubjectIdentity
	claimType  string
	variants   []claimVariant
}

type claimVariant struct {
	key         string
	description string
	metadata    []provenance.ClaimMetadata
}

type entityIndex struct {
	identity knowledge.EntityIdentity
	variants map[string]*entityVariant
}

type relationIndex struct {
	source   knowledge.EntityIdentity
	target   knowledge.EntityIdentity
	relType  string
	variants map[string]*relationVariant
}

type claimIndex struct {
	subjectKey string
	subject    knowledge.SubjectIdentity
	claimType  string
	variants   map[string]*claimVariant
}

func normalizeCommand(command Command) (sourceBatch, error) {
	if err := provenance.Validate(command.Source); err != nil {
		return sourceBatch{}, err
	}
	if len(command.Entities)+len(command.Relations)+len(command.Claims) == 0 {
		return sourceBatch{},
			fmt.Errorf("%w: Source Submission must contain Knowledge", knowledge.ErrInvalidChange)
	}

	normalized := sourceBatch{source: command.Source}
	entityBuilders := make(map[string]*entityIndex, len(command.Entities))
	for _, item := range command.Entities {
		identity, err := knowledge.NewEntityIdentity(item.Content.Identity.Title, item.Content.Identity.Type)
		if err != nil {
			return sourceBatch{}, err
		}
		aliases := canonicalStrings(item.Content.Aliases)
		content := knowledge.EntityContent{
			Identity: identity, Aliases: aliases, Description: item.Content.Description,
		}
		if err := knowledge.ValidateEntityContent(content); err != nil {
			return sourceBatch{}, err
		}
		metadata := item.Metadata
		metadata.Evidence = provenance.CanonicalEvidence(metadata.Evidence)
		if err := provenance.ValidateEntityMetadata(metadata); err != nil {
			return sourceBatch{}, err
		}
		if err := validateExtractionEvidence(command.Source.Kind, metadata.Evidence); err != nil {
			return sourceBatch{}, err
		}
		identityKey := canonicalKey("entity", identity.Title, identity.Type)
		contentKey := entityContentKey(aliases, item.Content.Description)
		group := entityBuilders[identityKey]
		if group == nil {
			group = &entityIndex{identity: identity, variants: make(map[string]*entityVariant)}
			entityBuilders[identityKey] = group
		}
		variant := group.variants[contentKey]
		if variant == nil {
			variant = &entityVariant{
				key: contentKey, aliases: aliases,
				description: item.Content.Description, metadata: metadata,
			}
			group.variants[contentKey] = variant
		} else if err := mergeEntityMetadata(&variant.metadata, metadata); err != nil {
			return sourceBatch{}, err
		}
	}
	normalized.entities = finishEntityGroups(entityBuilders)

	relationBuilders := make(map[string]*relationIndex, len(command.Relations))
	for _, item := range command.Relations {
		source, err := knowledge.NewEntityIdentity(item.Content.Source.Title, item.Content.Source.Type)
		if err != nil {
			return sourceBatch{}, err
		}
		target, err := knowledge.NewEntityIdentity(item.Content.Target.Title, item.Content.Target.Type)
		if err != nil {
			return sourceBatch{}, err
		}
		relationType := strings.TrimSpace(item.Content.Type)
		content := knowledge.RelationContent{
			Source: source, Target: target, Type: relationType,
			Description: item.Content.Description,
		}
		if err := knowledge.ValidateRelationContent(content); err != nil {
			return sourceBatch{}, err
		}
		metadata := item.Metadata
		metadata.Evidence = provenance.CanonicalEvidence(metadata.Evidence)
		if err := provenance.ValidateRelationMetadata(metadata); err != nil {
			return sourceBatch{}, err
		}
		if err := validateExtractionEvidence(command.Source.Kind, metadata.Evidence); err != nil {
			return sourceBatch{}, err
		}
		identityKey := canonicalKey(
			"relation",
			source.Title,
			source.Type,
			target.Title,
			target.Type,
			relationType,
		)
		contentKey := canonicalKey(item.Content.Description)
		group := relationBuilders[identityKey]
		if group == nil {
			group = &relationIndex{
				source: source, target: target, relType: relationType,
				variants: make(map[string]*relationVariant),
			}
			relationBuilders[identityKey] = group
		}
		variant := group.variants[contentKey]
		if variant == nil {
			variant = &relationVariant{
				key: contentKey, description: item.Content.Description, metadata: metadata,
			}
			group.variants[contentKey] = variant
		} else if err := mergeRelationMetadata(&variant.metadata, metadata); err != nil {
			return sourceBatch{}, err
		}
	}
	normalized.relations = finishRelationGroups(relationBuilders)

	claimBuilders := make(map[string]*claimIndex, len(command.Claims))
	for _, item := range command.Claims {
		subject, err := normalizeSubjectIdentity(item.Content.Subject)
		if err != nil {
			return sourceBatch{}, err
		}
		subjectKey, err := subjectIdentityKey(subject)
		if err != nil {
			return sourceBatch{}, err
		}
		claimType := strings.TrimSpace(item.Content.Type)
		content := knowledge.ClaimContent{
			Subject: subject, Type: claimType, Description: item.Content.Description,
		}
		if err := knowledge.ValidateClaimContent(content); err != nil {
			return sourceBatch{}, err
		}
		metadata := item.Metadata
		metadata.Evidence = provenance.CanonicalEvidence(metadata.Evidence)
		if err := provenance.ValidateClaimMetadata(metadata); err != nil {
			return sourceBatch{}, err
		}
		if command.Source.Kind == provenance.Extraction &&
			(strings.TrimSpace(metadata.SubjectText) == "" || strings.TrimSpace(metadata.SourceText) == "") {
			return sourceBatch{},
				fmt.Errorf("%w: Extraction Claim SubjectText and SourceText are required", provenance.ErrInvalidSource)
		}
		if err := validateExtractionEvidence(command.Source.Kind, metadata.Evidence); err != nil {
			return sourceBatch{}, err
		}
		identityKey := canonicalKey(subjectKey, claimType)
		contentKey := canonicalKey(item.Content.Description)
		group := claimBuilders[identityKey]
		if group == nil {
			group = &claimIndex{
				subjectKey: subjectKey, subject: subject,
				claimType: claimType, variants: make(map[string]*claimVariant),
			}
			claimBuilders[identityKey] = group
		}
		variant := group.variants[contentKey]
		if variant == nil {
			variant = &claimVariant{
				key: contentKey, description: item.Content.Description,
				metadata: []provenance.ClaimMetadata{metadata},
			}
			group.variants[contentKey] = variant
		} else {
			variant.metadata = append(variant.metadata, metadata)
		}
	}
	normalized.claims = finishClaimGroups(claimBuilders)

	evidence, err := sourceEvidence(normalized)
	if err != nil {
		return sourceBatch{}, err
	}
	normalized.evidence = evidence
	return normalized, nil
}

func normalizeSubjectIdentity(subject knowledge.SubjectIdentity) (knowledge.SubjectIdentity, error) {
	switch value := subject.(type) {
	case knowledge.EntityIdentity:
		identity, err := knowledge.NewEntityIdentity(value.Title, value.Type)
		return identity, err
	case knowledge.RelationIdentity:
		source, err := knowledge.NewEntityIdentity(value.Source.Title, value.Source.Type)
		if err != nil {
			return nil, err
		}
		target, err := knowledge.NewEntityIdentity(value.Target.Title, value.Target.Type)
		if err != nil {
			return nil, err
		}
		return knowledge.RelationIdentity{
			Source: source,
			Target: target,
			Type:   strings.TrimSpace(value.Type),
		}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported Claim Subject %T", knowledge.ErrInvalidChange, subject)
	}
}

func finishEntityGroups(builders map[string]*entityIndex) []entityBatch {
	keys := sortedEntityGroups(builders)
	groups := make([]entityBatch, 0, len(keys))
	for _, key := range keys {
		builder := builders[key]
		variantKeys := sortedEntityVariants(builder.variants)
		group := entityBatch{key: key, identity: builder.identity, variants: make([]entityVariant, 0, len(variantKeys))}
		for _, variantKey := range variantKeys {
			group.variants = append(group.variants, *builder.variants[variantKey])
		}
		groups = append(groups, group)
	}
	return groups
}

func finishRelationGroups(builders map[string]*relationIndex) []relationBatch {
	keys := sortedRelationGroups(builders)
	groups := make([]relationBatch, 0, len(keys))
	for _, key := range keys {
		builder := builders[key]
		variantKeys := sortedRelationVariants(builder.variants)
		group := relationBatch{
			key: key, source: builder.source, target: builder.target, relType: builder.relType,
			variants: make([]relationVariant, 0, len(variantKeys)),
		}
		for _, variantKey := range variantKeys {
			group.variants = append(group.variants, *builder.variants[variantKey])
		}
		groups = append(groups, group)
	}
	return groups
}

func finishClaimGroups(builders map[string]*claimIndex) []claimBatch {
	keys := sortedClaimGroups(builders)
	groups := make([]claimBatch, 0, len(keys))
	for _, key := range keys {
		builder := builders[key]
		variantKeys := sortedClaimVariants(builder.variants)
		group := claimBatch{
			key: key, subjectKey: builder.subjectKey,
			subject: builder.subject, claimType: builder.claimType,
			variants: make([]claimVariant, 0, len(variantKeys)),
		}
		for _, variantKey := range variantKeys {
			variant := *builder.variants[variantKey]
			slices.SortFunc(variant.metadata, compareClaimMetadata)
			variant.metadata = slices.CompactFunc(variant.metadata,
				func(left, right provenance.ClaimMetadata) bool {
					return compareClaimMetadata(left, right) == 0
				},
			)
			group.variants = append(group.variants, variant)
		}
		groups = append(groups, group)
	}
	return groups
}

func mergeEntityMetadata(target *provenance.EntityMetadata, addition provenance.EntityMetadata) error {
	maximum := int(^uint(0) >> 1)
	if addition.Frequency > maximum-target.Frequency {
		return fmt.Errorf("%w: Entity Source Frequency overflows int", provenance.ErrInvalidSource)
	}
	target.Frequency += addition.Frequency
	target.Evidence = provenance.CanonicalEvidence(append(target.Evidence, addition.Evidence...))
	return nil
}

func mergeRelationMetadata(target *provenance.RelationMetadata, addition provenance.RelationMetadata) error {
	target.Weight += addition.Weight
	if math.IsNaN(target.Weight) || math.IsInf(target.Weight, 0) {
		return fmt.Errorf("%w: Relation Source Weight sum must be finite", provenance.ErrInvalidSource)
	}
	target.Evidence = provenance.CanonicalEvidence(append(target.Evidence, addition.Evidence...))
	return nil
}

func compareClaimMetadata(left, right provenance.ClaimMetadata) int {
	return strings.Compare(claimMetadataKey(left), claimMetadataKey(right))
}

func validateExtractionEvidence(kind string, evidence []provenance.Evidence) error {
	if kind == provenance.Extraction && len(evidence) == 0 {
		return fmt.Errorf("%w: Extraction Source Metadata requires Evidence", provenance.ErrInvalidSource)
	}
	return nil
}

func canonicalStrings(values []string) []string {
	canonical := append([]string(nil), values...)
	slices.Sort(canonical)
	return slices.Compact(canonical)
}

func sortedEntityGroups(values map[string]*entityIndex) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedEntityVariants(values map[string]*entityVariant) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedRelationGroups(values map[string]*relationIndex) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedRelationVariants(values map[string]*relationVariant) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedClaimGroups(values map[string]*claimIndex) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedClaimVariants(values map[string]*claimVariant) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func canonicalKey(parts ...string) string {
	var key strings.Builder
	for _, part := range parts {
		key.WriteString(strconv.Itoa(len(part)))
		key.WriteByte(':')
		key.WriteString(part)
	}
	return key.String()
}

func entityContentKey(aliases []string, description string) string {
	parts := make([]string, 0, len(aliases)+2)
	parts = append(parts, description, strconv.Itoa(len(aliases)))
	parts = append(parts, aliases...)
	return canonicalKey(parts...)
}

func subjectIdentityKey(subject knowledge.SubjectIdentity) (string, error) {
	switch value := subject.(type) {
	case knowledge.EntityIdentity:
		return canonicalKey("entity", value.Title, value.Type), nil
	case knowledge.RelationIdentity:
		return canonicalKey(
			"relation",
			value.Source.Title,
			value.Source.Type,
			value.Target.Title,
			value.Target.Type,
			value.Type,
		), nil
	default:
		return "", fmt.Errorf("%w: unsupported Claim Subject %T", knowledge.ErrInvalidChange, subject)
	}
}

func claimMetadataKey(metadata provenance.ClaimMetadata) string {
	parts := []string{
		metadata.SubjectText,
		metadata.ObjectText,
		metadata.Status,
		metadata.StartDate,
		metadata.EndDate,
		metadata.SourceText,
	}
	for _, evidence := range metadata.Evidence {
		parts = append(parts, evidence.ZoneID, evidence.TextUnitID)
		switch source := evidence.Source.(type) {
		case provenance.CorporaSource:
			parts = append(parts, "corpora", source.CorporaID)
		case provenance.MessageSource:
			parts = append(parts, "message", source.MessageID)
		}
	}
	return canonicalKey(parts...)
}

func sourceEvidence(command sourceBatch) ([]provenance.Evidence, error) {
	var evidence []provenance.Evidence
	set := false
	accept := func(candidate []provenance.Evidence) error {
		candidate = provenance.CanonicalEvidence(candidate)
		if !set {
			evidence = candidate
			set = true
			return nil
		}
		if !slices.Equal(evidence, candidate) {
			return fmt.Errorf("%w: all Knowledge from one Source must share the same Evidence", provenance.ErrInvalidSource)
		}
		return nil
	}
	for _, group := range command.entities {
		for _, variant := range group.variants {
			if err := accept(variant.metadata.Evidence); err != nil {
				return nil, err
			}
		}
	}
	for _, group := range command.relations {
		for _, variant := range group.variants {
			if err := accept(variant.metadata.Evidence); err != nil {
				return nil, err
			}
		}
	}
	for _, group := range command.claims {
		for _, variant := range group.variants {
			for _, metadata := range variant.metadata {
				if err := accept(metadata.Evidence); err != nil {
					return nil, err
				}
			}
		}
	}
	return evidence, nil
}
