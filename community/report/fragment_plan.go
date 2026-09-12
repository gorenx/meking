package report

import (
	"errors"
	"fmt"
	"sort"

	"github.com/memoria-space/meking/community"
)

func planReportFragments(
	root community.Membership,
	communities []community.Membership,
	input communityInput,
	entitiesByID map[string]Entity,
	tokens community.ReportTokenCounter,
	config Config,
) ([]string, error) {
	children := make(map[community.CommunityID][]community.Membership)
	for _, current := range communities {
		if current.ParentID != nil {
			children[*current.ParentID] = append(children[*current.ParentID], current)
		}
	}
	for id := range children {
		sort.Slice(children[id], func(left, right int) bool {
			return children[id][left].Number < children[id][right].Number
		})
	}

	complete := numberReportInput(input)
	candidates, err := splitReportRegion(
		root,
		complete,
		false,
		children,
		entitiesByID,
		tokens,
		config,
	)
	if err != nil {
		return nil, fmt.Errorf("plan Report fragments for Community %q: %w", root.ID, err)
	}
	candidates, err = combineReportCandidates(candidates, entitiesByID, tokens, config)
	if err != nil {
		return nil, fmt.Errorf("combine Report fragments for Community %q: %w", root.ID, err)
	}
	if err := validateBatchCoverage(complete, candidates); err != nil {
		return nil, fmt.Errorf("validate Report fragments for Community %q: %w", root.ID, err)
	}

	prompts := make([]string, len(candidates))
	for index, candidate := range candidates {
		prompt, fits, err := fragmentPromptForBatch(candidate, entitiesByID, tokens, config)
		if err != nil {
			return nil, err
		}
		if !fits {
			return nil, fmt.Errorf("%w: planned evidence batch %d exceeds %d tokens",
				ErrReportInputTooLarge, index, config.MaxInputTokens)
		}
		prompts[index] = prompt
	}
	return prompts, nil
}

func splitReportRegion(
	current community.Membership,
	input reportBatch,
	allowWhole bool,
	children map[community.CommunityID][]community.Membership,
	entitiesByID map[string]Entity,
	tokens community.ReportTokenCounter,
	config Config,
) ([]reportBatch, error) {
	if allowWhole {
		_, fits, err := fragmentPromptForBatch(input, entitiesByID, tokens, config)
		if err != nil {
			return nil, err
		}
		if fits {
			return []reportBatch{input}, nil
		}
	}

	direct := children[current.ID]
	if len(direct) == 0 {
		return packReportEvidence(input, entitiesByID, tokens, config)
	}
	owned := reportBatch{}
	result := make([]reportBatch, 0, len(direct)+1)
	for _, child := range direct {
		subset := reportBatchForMembers(input, child.EntityIDs)
		if reportBatchEmpty(subset) {
			return nil, fmt.Errorf("child Community %q owns no evidence", child.ID)
		}
		parts, err := splitReportRegion(
			child,
			subset,
			true,
			children,
			entitiesByID,
			tokens,
			config,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, parts...)
		owned = mergeReportBatches(owned, subset)
	}
	remainder := subtractReportBatch(input, owned)
	if !reportBatchEmpty(remainder) {
		parts, err := packReportEvidence(remainder, entitiesByID, tokens, config)
		if err != nil {
			return nil, err
		}
		result = append(result, parts...)
	}
	return result, nil
}

func reportBatchForMembers(input reportBatch, entityIDs []string) reportBatch {
	members := make(map[string]struct{}, len(entityIDs))
	for _, id := range entityIDs {
		members[id] = struct{}{}
	}
	result := reportBatch{}
	for _, item := range input.entities {
		if _, found := members[item.value.ID]; found {
			result.entities = append(result.entities, item)
		}
	}
	ownedRelations := make(map[string]struct{})
	for _, item := range input.relations {
		_, sourceFound := members[item.value.SourceEntityID]
		_, targetFound := members[item.value.TargetEntityID]
		if sourceFound && targetFound {
			result.relations = append(result.relations, item)
			ownedRelations[item.value.ID] = struct{}{}
		}
	}
	for _, item := range input.claims {
		switch item.value.Subject.Kind {
		case EntityClaimSubject:
			if _, found := members[item.value.Subject.ID]; found {
				result.claims = append(result.claims, item)
			}
		case RelationClaimSubject:
			if _, found := ownedRelations[item.value.Subject.ID]; found {
				result.claims = append(result.claims, item)
			}
		}
	}
	return result
}

func subtractReportBatch(input, owned reportBatch) reportBatch {
	entities := make(map[int]struct{}, len(owned.entities))
	relations := make(map[int]struct{}, len(owned.relations))
	claims := make(map[int]struct{}, len(owned.claims))
	for _, item := range owned.entities {
		entities[item.row] = struct{}{}
	}
	for _, item := range owned.relations {
		relations[item.row] = struct{}{}
	}
	for _, item := range owned.claims {
		claims[item.row] = struct{}{}
	}
	result := reportBatch{}
	for _, item := range input.entities {
		if _, found := entities[item.row]; !found {
			result.entities = append(result.entities, item)
		}
	}
	for _, item := range input.relations {
		if _, found := relations[item.row]; !found {
			result.relations = append(result.relations, item)
		}
	}
	for _, item := range input.claims {
		if _, found := claims[item.row]; !found {
			result.claims = append(result.claims, item)
		}
	}
	return result
}

func packReportEvidence(
	input reportBatch,
	entitiesByID map[string]Entity,
	tokens community.ReportTokenCounter,
	config Config,
) ([]reportBatch, error) {
	atoms := reportEvidenceAtoms(input)
	result := make([]reportBatch, 0, len(atoms))
	current := reportBatch{}
	for _, atom := range atoms {
		candidate := mergeReportBatches(current, atom.batch)
		_, fits, err := fragmentPromptForBatch(candidate, entitiesByID, tokens, config)
		if err != nil {
			return nil, err
		}
		if fits {
			current = candidate
			continue
		}
		if reportBatchEmpty(current) {
			return nil, fmt.Errorf(
				"%w: %s %q cannot fit one fragment prompt within %d tokens",
				ErrReportInputTooLarge,
				atom.kind,
				atom.id,
				config.MaxInputTokens,
			)
		}
		result = append(result, current)
		_, atomFits, err := fragmentPromptForBatch(atom.batch, entitiesByID, tokens, config)
		if err != nil {
			return nil, err
		}
		if !atomFits {
			return nil, fmt.Errorf(
				"%w: %s %q cannot fit one fragment prompt within %d tokens",
				ErrReportInputTooLarge,
				atom.kind,
				atom.id,
				config.MaxInputTokens,
			)
		}
		current = atom.batch
	}
	if !reportBatchEmpty(current) {
		result = append(result, current)
	}
	return result, nil
}

type reportEvidenceAtom struct {
	kind  string
	id    string
	batch reportBatch
}

func reportEvidenceAtoms(input reportBatch) []reportEvidenceAtom {
	claimsBySubject := make(map[ClaimSubject][]numberedClaim)
	for _, item := range input.claims {
		claimsBySubject[item.value.Subject] = append(claimsBySubject[item.value.Subject], item)
	}
	atoms := make([]reportEvidenceAtom, 0, len(input.entities)+len(input.relations)+len(input.claims))
	usedClaims := make(map[int]struct{}, len(input.claims))
	for _, entity := range input.entities {
		atoms = append(atoms, reportEvidenceAtom{
			kind: "Entity", id: entity.value.ID,
			batch: reportBatch{entities: []numberedEntity{entity}},
		})
		for _, claim := range claimsBySubject[ClaimSubject{Kind: EntityClaimSubject, ID: entity.value.ID}] {
			atoms = append(atoms, reportEvidenceAtom{
				kind: "Claim", id: claim.value.ID,
				batch: reportBatch{claims: []numberedClaim{claim}},
			})
			usedClaims[claim.row] = struct{}{}
		}
	}
	for _, relation := range input.relations {
		atoms = append(atoms, reportEvidenceAtom{
			kind: "Relation", id: relation.value.ID,
			batch: reportBatch{relations: []numberedRelation{relation}},
		})
		for _, claim := range claimsBySubject[ClaimSubject{Kind: RelationClaimSubject, ID: relation.value.ID}] {
			atoms = append(atoms, reportEvidenceAtom{
				kind: "Claim", id: claim.value.ID,
				batch: reportBatch{claims: []numberedClaim{claim}},
			})
			usedClaims[claim.row] = struct{}{}
		}
	}
	for _, claim := range input.claims {
		if _, used := usedClaims[claim.row]; !used {
			atoms = append(atoms, reportEvidenceAtom{
				kind: "Claim", id: claim.value.ID,
				batch: reportBatch{claims: []numberedClaim{claim}},
			})
		}
	}
	return atoms
}

func combineReportCandidates(
	candidates []reportBatch,
	entitiesByID map[string]Entity,
	tokens community.ReportTokenCounter,
	config Config,
) ([]reportBatch, error) {
	if len(candidates) == 0 {
		return nil, errors.New("Report fragment plan contains no evidence batches")
	}
	result := make([]reportBatch, 0, len(candidates))
	current := candidates[0]
	for _, next := range candidates[1:] {
		combined := mergeReportBatches(current, next)
		_, fits, err := fragmentPromptForBatch(combined, entitiesByID, tokens, config)
		if err != nil {
			return nil, err
		}
		if fits {
			current = combined
			continue
		}
		result = append(result, current)
		current = next
	}
	return append(result, current), nil
}

func fragmentPromptForBatch(
	batch reportBatch,
	entitiesByID map[string]Entity,
	tokens community.ReportTokenCounter,
	config Config,
) (string, bool, error) {
	context, err := renderReportBatch(batch, entitiesByID)
	if err != nil {
		return "", false, err
	}
	prompt := renderFragmentPrompt(context, fragmentWordLimit(config))
	count, err := tokens.Count(prompt)
	if err != nil {
		return "", false, fmt.Errorf("count Report fragment prompt: %w", err)
	}
	if count < 0 {
		return "", false, errors.New("count Report fragment prompt: token counter returned a negative count")
	}
	return prompt, count <= config.MaxInputTokens, nil
}

func validateBatchCoverage(complete reportBatch, batches []reportBatch) error {
	entityRows := make(map[int]int, len(complete.entities))
	relationRows := make(map[int]int, len(complete.relations))
	claimRows := make(map[int]int, len(complete.claims))
	for _, batch := range batches {
		for _, item := range batch.entities {
			entityRows[item.row]++
		}
		for _, item := range batch.relations {
			relationRows[item.row]++
		}
		for _, item := range batch.claims {
			claimRows[item.row]++
		}
	}
	for _, item := range complete.entities {
		if entityRows[item.row] != 1 {
			return fmt.Errorf("Entity row %d is owned %d times", item.row, entityRows[item.row])
		}
	}
	for _, item := range complete.relations {
		if relationRows[item.row] != 1 {
			return fmt.Errorf("Relation row %d is owned %d times", item.row, relationRows[item.row])
		}
	}
	for _, item := range complete.claims {
		if claimRows[item.row] != 1 {
			return fmt.Errorf("Claim row %d is owned %d times", item.row, claimRows[item.row])
		}
	}
	return nil
}
