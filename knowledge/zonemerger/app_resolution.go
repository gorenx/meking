package zonemerger

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/zone"
)

var errParentChanged = errors.New("Parent formal Knowledge changed after Child conflict assignment")

func (application *Application) ResolveEntity(
	ctx context.Context,
	command resolution.EntityCommand,
) (resolution.Result, error) {
	assignment, found, err := application.Conflicts.EntityConflict(ctx, command.Final.ID, command.BaseVersion)
	if err != nil {
		return resolution.Result{}, err
	}
	if !found {
		return application.resumeEntityResolution(ctx, command)
	}
	if err := requireChildZone(ctx, assignment.ChildZoneID); err != nil {
		return resolution.Result{}, err
	}
	result, err := application.resolveEntityWithParent(ctx, assignment, command)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errParentChanged) {
		return resolution.Result{}, classifyConflictChange(err)
	}
	return application.resolveEntityAfterParentChange(ctx, assignment, command)
}

func (application *Application) ResolveRelation(
	ctx context.Context,
	command resolution.RelationCommand,
) (resolution.Result, error) {
	assignment, found, err := application.Conflicts.RelationConflict(ctx, command.Final.ID, command.BaseVersion)
	if err != nil {
		return resolution.Result{}, err
	}
	if !found {
		return application.resumeRelationResolution(ctx, command)
	}
	if err := requireChildZone(ctx, assignment.ChildZoneID); err != nil {
		return resolution.Result{}, err
	}
	result, err := application.resolveRelationWithParent(ctx, assignment, command)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errParentChanged) {
		return resolution.Result{}, classifyConflictChange(err)
	}
	return application.resolveRelationAfterParentChange(ctx, assignment, command)
}

func (application *Application) ResolveClaim(
	ctx context.Context,
	command resolution.ClaimCommand,
) (resolution.Result, error) {
	assignment, found, err := application.Conflicts.ClaimConflict(ctx, command.Final.ID, command.BaseVersion)
	if err != nil {
		return resolution.Result{}, err
	}
	if !found {
		return application.resumeClaimResolution(ctx, command)
	}
	if err := requireChildZone(ctx, assignment.ChildZoneID); err != nil {
		return resolution.Result{}, err
	}
	result, err := application.resolveClaimWithParent(ctx, assignment, command)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errParentChanged) {
		return resolution.Result{}, classifyConflictChange(err)
	}
	return application.resolveClaimAfterParentChange(ctx, assignment, command)
}

func (application *Application) resolveEntityWithParent(
	ctx context.Context,
	assignment EntityConflict,
	command resolution.EntityCommand,
) (resolution.Result, error) {
	var childResult resolution.Result
	err := application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		parentContext, err := zone.RouteContext(transactionContext, assignment.ParentZoneID)
		if err != nil {
			return err
		}
		parent, err := application.currentParentEntity(parentContext, assignment)
		if err != nil {
			return err
		}
		childResult, err = application.Resolutions.ResolveEntity(transactionContext, command)
		if err != nil {
			return err
		}
		if err := application.publishParentEntity(parentContext, assignment, parent, command); err != nil {
			return err
		}
		if err := application.Conflicts.ResolveEntityConflict(transactionContext, assignment, command.Source.ID); err != nil {
			return err
		}
		return application.Conflicts.ReconcileEntityConflict(transactionContext, assignment, command.Source.ID)
	})
	return childResult, err
}

func (application *Application) resolveRelationWithParent(
	ctx context.Context,
	assignment RelationConflict,
	command resolution.RelationCommand,
) (resolution.Result, error) {
	var childResult resolution.Result
	err := application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		parentContext, err := zone.RouteContext(transactionContext, assignment.ParentZoneID)
		if err != nil {
			return err
		}
		parent, err := application.currentParentRelation(parentContext, assignment)
		if err != nil {
			return err
		}
		childResult, err = application.Resolutions.ResolveRelation(transactionContext, command)
		if err != nil {
			return err
		}
		if err := application.publishParentRelation(parentContext, assignment, parent, command); err != nil {
			return err
		}
		if err := application.Conflicts.ResolveRelationConflict(transactionContext, assignment, command.Source.ID); err != nil {
			return err
		}
		return application.Conflicts.ReconcileRelationConflict(transactionContext, assignment, command.Source.ID)
	})
	return childResult, err
}

func (application *Application) resolveClaimWithParent(
	ctx context.Context,
	assignment ClaimConflict,
	command resolution.ClaimCommand,
) (resolution.Result, error) {
	var childResult resolution.Result
	err := application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		parentContext, err := zone.RouteContext(transactionContext, assignment.ParentZoneID)
		if err != nil {
			return err
		}
		parent, err := application.currentParentClaim(parentContext, assignment)
		if err != nil {
			return err
		}
		childResult, err = application.Resolutions.ResolveClaim(transactionContext, command)
		if err != nil {
			return err
		}
		if err := application.publishParentClaim(parentContext, assignment, parent, command); err != nil {
			return err
		}
		if err := application.Conflicts.ResolveClaimConflict(transactionContext, assignment, command.Source.ID); err != nil {
			return err
		}
		return application.Conflicts.ReconcileClaimConflict(transactionContext, assignment, command.Source.ID)
	})
	return childResult, err
}

func (application *Application) currentParentEntity(
	ctx context.Context,
	assignment EntityConflict,
) (knowledge.KnowledgeVersion[knowledge.Entity], error) {
	current, found, err := application.CurrentVersions.CurrentEntity(ctx, assignment.ParentEntityID)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, err
	}
	if !found || current.Deleted || current.Version != assignment.ParentVersion || current.Hash != assignment.CandidateHash {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, errParentChanged
	}
	if err := application.requireNoEntityConflict(ctx, assignment.ParentEntityID); err != nil {
		return knowledge.KnowledgeVersion[knowledge.Entity]{}, err
	}
	return current, nil
}

func (application *Application) currentParentRelation(
	ctx context.Context,
	assignment RelationConflict,
) (knowledge.KnowledgeVersion[knowledge.Relation], error) {
	current, found, err := application.CurrentVersions.CurrentRelation(ctx, assignment.ParentRelationID)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, err
	}
	if !found || current.Deleted || current.Version != assignment.ParentVersion || current.Hash != assignment.CandidateHash {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, errParentChanged
	}
	if err := application.requireNoRelationConflict(ctx, assignment.ParentRelationID); err != nil {
		return knowledge.KnowledgeVersion[knowledge.Relation]{}, err
	}
	return current, nil
}

func (application *Application) currentParentClaim(
	ctx context.Context,
	assignment ClaimConflict,
) (knowledge.KnowledgeVersion[knowledge.Claim], error) {
	current, found, err := application.CurrentVersions.CurrentClaim(ctx, assignment.ParentClaimID)
	if err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, err
	}
	if !found || current.Deleted || current.Version != assignment.ParentVersion || current.Hash != assignment.CandidateHash {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, errParentChanged
	}
	if err := application.requireNoClaimConflict(ctx, assignment.ParentClaimID); err != nil {
		return knowledge.KnowledgeVersion[knowledge.Claim]{}, err
	}
	return current, nil
}

func (application *Application) requireNoEntityConflict(ctx context.Context, id knowledge.EntityID) error {
	_, err := application.Resolutions.EntityConflict(ctx, id)
	if err == nil {
		return errParentChanged
	}
	if errors.Is(err, knowledge.ErrNotFound) {
		return nil
	}
	return err
}

func (application *Application) requireNoRelationConflict(ctx context.Context, id knowledge.RelationID) error {
	_, err := application.Resolutions.RelationConflict(ctx, id)
	if err == nil {
		return errParentChanged
	}
	if errors.Is(err, knowledge.ErrNotFound) {
		return nil
	}
	return err
}

func (application *Application) requireNoClaimConflict(ctx context.Context, id knowledge.ClaimID) error {
	_, err := application.Resolutions.ClaimConflict(ctx, id)
	if err == nil {
		return errParentChanged
	}
	if errors.Is(err, knowledge.ErrNotFound) {
		return nil
	}
	return err
}

func (application *Application) publishParentEntity(
	ctx context.Context,
	assignment EntityConflict,
	parent knowledge.KnowledgeVersion[knowledge.Entity],
	command resolution.EntityCommand,
) error {
	digest, err := entityAssignmentDigest(assignment)
	if err != nil {
		return err
	}
	support, err := parentSupportSource(command.Source, digest)
	if err != nil {
		return err
	}
	final := knowledge.Entity{
		ID:          assignment.ParentEntityID,
		Title:       parent.Knowledge.Title,
		Type:        parent.Knowledge.Type,
		Aliases:     append([]string(nil), command.Final.Aliases...),
		Description: command.Final.Description,
	}
	result, err := application.Submissions.Submit(ctx, submission.Command{
		Source: support,
		Entities: []submission.Entity{
			{
				Content: knowledge.EntityContent{
					Identity: knowledge.EntityIdentity{
						Title: final.Title,
						Type:  final.Type,
					},
					Aliases:     append([]string(nil), final.Aliases...),
					Description: final.Description,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if len(result.OpenedCandidates.Entities)+len(result.ReusedCandidates.Entities) == 0 {
		return nil
	}
	parentConflict, err := application.Resolutions.EntityConflict(ctx, assignment.ParentEntityID)
	if err != nil {
		return err
	}
	decision, err := parentDecisionSource(command.Source, digest)
	if err != nil {
		return err
	}
	_, err = application.Resolutions.ResolveEntity(ctx, resolution.EntityCommand{
		Source:      decision,
		BaseVersion: parentConflict.BaseVersion,
		Final:       final,
		Candidates:  entityExpectations(parentConflict),
	})
	return err
}

func (application *Application) publishParentRelation(
	ctx context.Context,
	assignment RelationConflict,
	parent knowledge.KnowledgeVersion[knowledge.Relation],
	command resolution.RelationCommand,
) error {
	digest, err := relationAssignmentDigest(assignment)
	if err != nil {
		return err
	}
	support, err := parentSupportSource(command.Source, digest)
	if err != nil {
		return err
	}
	sourceIdentity, targetIdentity, err := application.parentEntityIdentities(ctx, parent.Knowledge)
	if err != nil {
		return err
	}
	final := knowledge.Relation{
		ID:             assignment.ParentRelationID,
		SourceEntityID: parent.Knowledge.SourceEntityID,
		TargetEntityID: parent.Knowledge.TargetEntityID,
		Type:           parent.Knowledge.Type,
		Description:    command.Final.Description,
	}
	result, err := application.Submissions.Submit(ctx, submission.Command{
		Source: support,
		Relations: []submission.Relation{
			{
				Content: knowledge.RelationContent{
					Source:      sourceIdentity,
					Target:      targetIdentity,
					Type:        final.Type,
					Description: final.Description,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if len(result.OpenedCandidates.Relations)+len(result.ReusedCandidates.Relations) == 0 {
		return nil
	}
	parentConflict, err := application.Resolutions.RelationConflict(ctx, assignment.ParentRelationID)
	if err != nil {
		return err
	}
	decision, err := parentDecisionSource(command.Source, digest)
	if err != nil {
		return err
	}
	_, err = application.Resolutions.ResolveRelation(ctx, resolution.RelationCommand{
		Source:      decision,
		BaseVersion: parentConflict.BaseVersion,
		Final:       final,
		Candidates:  relationExpectations(parentConflict),
	})
	return err
}

func (application *Application) publishParentClaim(
	ctx context.Context,
	assignment ClaimConflict,
	parent knowledge.KnowledgeVersion[knowledge.Claim],
	command resolution.ClaimCommand,
) error {
	digest, err := claimAssignmentDigest(assignment)
	if err != nil {
		return err
	}
	support, err := parentSupportSource(command.Source, digest)
	if err != nil {
		return err
	}
	subjectIdentity, err := application.parentSubjectIdentity(ctx, parent.Knowledge.Subject)
	if err != nil {
		return err
	}
	final := knowledge.Claim{
		ID:          assignment.ParentClaimID,
		Subject:     parent.Knowledge.Subject,
		Type:        parent.Knowledge.Type,
		Description: command.Final.Description,
	}
	result, err := application.Submissions.Submit(ctx, submission.Command{
		Source: support,
		Claims: []submission.Claim{
			{
				Content: knowledge.ClaimContent{
					Subject:     subjectIdentity,
					Type:        final.Type,
					Description: final.Description,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if len(result.OpenedCandidates.Claims)+len(result.ReusedCandidates.Claims) == 0 {
		return nil
	}
	parentConflict, err := application.Resolutions.ClaimConflict(ctx, assignment.ParentClaimID)
	if err != nil {
		return err
	}
	decision, err := parentDecisionSource(command.Source, digest)
	if err != nil {
		return err
	}
	_, err = application.Resolutions.ResolveClaim(ctx, resolution.ClaimCommand{
		Source:      decision,
		BaseVersion: parentConflict.BaseVersion,
		Final:       final,
		Candidates:  claimExpectations(parentConflict),
	})
	return err
}

func (application *Application) parentEntityIdentities(
	ctx context.Context,
	relation knowledge.Relation,
) (knowledge.EntityIdentity, knowledge.EntityIdentity, error) {
	source, found, err := application.Identities.EntityIdentity(ctx, relation.SourceEntityID)
	if err != nil || !found {
		return knowledge.EntityIdentity{}, knowledge.EntityIdentity{}, missingParentIdentity("Relation Source Entity", err)
	}
	target, found, err := application.Identities.EntityIdentity(ctx, relation.TargetEntityID)
	if err != nil || !found {
		return knowledge.EntityIdentity{}, knowledge.EntityIdentity{}, missingParentIdentity("Relation Target Entity", err)
	}
	return source, target, nil
}

func (application *Application) parentSubjectIdentity(
	ctx context.Context,
	subject knowledge.Subject,
) (knowledge.SubjectIdentity, error) {
	switch id := subject.(type) {
	case knowledge.EntityID:
		identity, found, err := application.Identities.EntityIdentity(ctx, id)
		if err != nil || !found {
			return nil, missingParentIdentity("Claim Entity Subject", err)
		}
		return identity, nil
	case knowledge.RelationID:
		identity, found, err := application.Identities.RelationKey(ctx, id)
		if err != nil || !found {
			return nil, missingParentIdentity("Claim Relation Subject", err)
		}
		source, target, err := application.parentEntityIdentities(ctx, knowledge.Relation{
			SourceEntityID: identity.SourceEntityID,
			TargetEntityID: identity.TargetEntityID,
		})
		if err != nil {
			return nil, err
		}
		return knowledge.RelationIdentity{
			Source: source,
			Target: target,
			Type:   identity.Type,
		}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported Parent Claim subject %T", knowledge.ErrDataIntegrity, subject)
	}
}

func (application *Application) resolveEntityAfterParentChange(
	ctx context.Context,
	assignment EntityConflict,
	command resolution.EntityCommand,
) (resolution.Result, error) {
	var result resolution.Result
	err := application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		var err error
		result, err = application.Resolutions.ResolveEntity(transactionContext, command)
		if err != nil {
			return err
		}
		return application.Conflicts.ResolveEntityConflict(transactionContext, assignment, command.Source.ID)
	})
	if err != nil {
		return resolution.Result{}, classifyConflictChange(err)
	}
	if err := application.mergeResolvedChild(ctx, assignment.ChildZoneID, assignment.ParentZoneID); err != nil {
		return resolution.Result{}, err
	}
	err = application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		return application.Conflicts.ReconcileEntityConflict(transactionContext, assignment, command.Source.ID)
	})
	return result, err
}

func (application *Application) resolveRelationAfterParentChange(
	ctx context.Context,
	assignment RelationConflict,
	command resolution.RelationCommand,
) (resolution.Result, error) {
	var result resolution.Result
	err := application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		var err error
		result, err = application.Resolutions.ResolveRelation(transactionContext, command)
		if err != nil {
			return err
		}
		return application.Conflicts.ResolveRelationConflict(transactionContext, assignment, command.Source.ID)
	})
	if err != nil {
		return resolution.Result{}, classifyConflictChange(err)
	}
	if err := application.mergeResolvedChild(ctx, assignment.ChildZoneID, assignment.ParentZoneID); err != nil {
		return resolution.Result{}, err
	}
	err = application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		return application.Conflicts.ReconcileRelationConflict(transactionContext, assignment, command.Source.ID)
	})
	return result, err
}

func (application *Application) resolveClaimAfterParentChange(
	ctx context.Context,
	assignment ClaimConflict,
	command resolution.ClaimCommand,
) (resolution.Result, error) {
	var result resolution.Result
	err := application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		var err error
		result, err = application.Resolutions.ResolveClaim(transactionContext, command)
		if err != nil {
			return err
		}
		return application.Conflicts.ResolveClaimConflict(transactionContext, assignment, command.Source.ID)
	})
	if err != nil {
		return resolution.Result{}, classifyConflictChange(err)
	}
	if err := application.mergeResolvedChild(ctx, assignment.ChildZoneID, assignment.ParentZoneID); err != nil {
		return resolution.Result{}, err
	}
	err = application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		return application.Conflicts.ReconcileClaimConflict(transactionContext, assignment, command.Source.ID)
	})
	return result, err
}

func (application *Application) resumeEntityResolution(ctx context.Context, command resolution.EntityCommand) (resolution.Result, error) {
	result, err := application.Resolutions.EntityResult(ctx, command)
	if err != nil {
		if errors.Is(err, resolution.ErrResolutionNotFound) {
			return resolution.Result{}, knowledge.ErrNotFound
		}
		return resolution.Result{}, err
	}
	assignment, err := application.Conflicts.ResolvedEntityConflict(ctx, command.Source.ID)
	if err != nil {
		return resolution.Result{}, err
	}
	if assignment.ChildEntityID != command.Final.ID || assignment.ChildBase != command.BaseVersion {
		return resolution.Result{}, fmt.Errorf("%w: Resolution Source is bound to another Child Entity conflict", provenance.ErrInvalidSource)
	}
	if assignment.ParentReconciled {
		return result, nil
	}
	if err := application.mergeResolvedChild(ctx, assignment.ChildZoneID, assignment.ParentZoneID); err != nil {
		return resolution.Result{}, err
	}
	err = application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		return application.Conflicts.ReconcileEntityConflict(transactionContext, assignment, command.Source.ID)
	})
	return result, err
}

func (application *Application) resumeRelationResolution(ctx context.Context, command resolution.RelationCommand) (resolution.Result, error) {
	result, err := application.Resolutions.RelationResult(ctx, command)
	if err != nil {
		if errors.Is(err, resolution.ErrResolutionNotFound) {
			return resolution.Result{}, knowledge.ErrNotFound
		}
		return resolution.Result{}, err
	}
	assignment, err := application.Conflicts.ResolvedRelationConflict(ctx, command.Source.ID)
	if err != nil {
		return resolution.Result{}, err
	}
	if assignment.ChildRelationID != command.Final.ID || assignment.ChildBase != command.BaseVersion {
		return resolution.Result{}, fmt.Errorf("%w: Resolution Source is bound to another Child Relation conflict", provenance.ErrInvalidSource)
	}
	if assignment.ParentReconciled {
		return result, nil
	}
	if err := application.mergeResolvedChild(ctx, assignment.ChildZoneID, assignment.ParentZoneID); err != nil {
		return resolution.Result{}, err
	}
	err = application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		return application.Conflicts.ReconcileRelationConflict(transactionContext, assignment, command.Source.ID)
	})
	return result, err
}

func (application *Application) resumeClaimResolution(ctx context.Context, command resolution.ClaimCommand) (resolution.Result, error) {
	result, err := application.Resolutions.ClaimResult(ctx, command)
	if err != nil {
		if errors.Is(err, resolution.ErrResolutionNotFound) {
			return resolution.Result{}, knowledge.ErrNotFound
		}
		return resolution.Result{}, err
	}
	assignment, err := application.Conflicts.ResolvedClaimConflict(ctx, command.Source.ID)
	if err != nil {
		return resolution.Result{}, err
	}
	if assignment.ChildClaimID != command.Final.ID || assignment.ChildBase != command.BaseVersion {
		return resolution.Result{}, fmt.Errorf("%w: Resolution Source is bound to another Child Claim conflict", provenance.ErrInvalidSource)
	}
	if assignment.ParentReconciled {
		return result, nil
	}
	if err := application.mergeResolvedChild(ctx, assignment.ChildZoneID, assignment.ParentZoneID); err != nil {
		return resolution.Result{}, err
	}
	err = application.Tx.WithTx(ctx, func(transactionContext context.Context) error {
		return application.Conflicts.ReconcileClaimConflict(transactionContext, assignment, command.Source.ID)
	})
	return result, err
}

func (application *Application) mergeResolvedChild(ctx context.Context, childZoneID zone.ID, parentZoneID zone.ID) error {
	parentContext, err := zone.RouteContext(ctx, parentZoneID)
	if err != nil {
		return err
	}
	mergeContext, err := zone.WithChildZone(parentContext, childZoneID)
	if err != nil {
		return err
	}
	return application.MergeChildKnowledge(mergeContext)
}

func entityExpectations(value resolution.EntityConflict) []resolution.EntityExpectation {
	result := make([]resolution.EntityExpectation, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		expectation := resolution.EntityExpectation{
			Candidate: candidate.Candidate.Key,
		}
		for _, source := range candidate.Sources {
			expectation.SourceIDs = append(expectation.SourceIDs, source.SourceID)
		}
		if len(expectation.SourceIDs) > 0 {
			result = append(result, expectation)
		}
	}
	return result
}

func relationExpectations(value resolution.RelationConflict) []resolution.RelationExpectation {
	result := make([]resolution.RelationExpectation, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		expectation := resolution.RelationExpectation{
			Candidate: candidate.Candidate.Key,
		}
		for _, source := range candidate.Sources {
			expectation.SourceIDs = append(expectation.SourceIDs, source.SourceID)
		}
		if len(expectation.SourceIDs) > 0 {
			result = append(result, expectation)
		}
	}
	return result
}

func claimExpectations(value resolution.ClaimConflict) []resolution.ClaimExpectation {
	result := make([]resolution.ClaimExpectation, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		expectation := resolution.ClaimExpectation{
			Candidate: candidate.Candidate.Key,
		}
		for _, source := range candidate.Sources {
			expectation.SourceIDs = append(expectation.SourceIDs, source.SourceID)
		}
		if len(expectation.SourceIDs) > 0 {
			result = append(result, expectation)
		}
	}
	return result
}

func requireChildZone(ctx context.Context, expected zone.ID) error {
	actual, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%w: conflict belongs to Child Zone %q, current Zone is %q", ErrInvalidMerge, expected, actual)
	}
	return nil
}

func classifyConflictChange(err error) error {
	if errors.Is(err, resolution.ErrConflictChanged) {
		return errors.Join(ErrConflictChanged, err)
	}
	return err
}
