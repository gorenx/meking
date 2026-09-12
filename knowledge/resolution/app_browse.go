package resolution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/memoria-space/meking/knowledge"
)

func (application *Application) BrowseEntityConflicts(
	ctx context.Context,
	request ConflictPageRequest,
) (EntityConflictPage, error) {
	if err := requireConflictApplication(application); err != nil {
		return EntityConflictPage{}, err
	}
	request, err := normalizeConflictPage(request)
	if err != nil {
		return EntityConflictPage{}, err
	}
	return browseConflicts(
		ctx,
		request,
		application.Candidates.ListEntityIDs,
		application.EntityConflict,
	)
}

func (application *Application) BrowseRelationConflicts(
	ctx context.Context,
	request ConflictPageRequest,
) (RelationConflictPage, error) {
	if err := requireConflictApplication(application); err != nil {
		return RelationConflictPage{}, err
	}
	request, err := normalizeConflictPage(request)
	if err != nil {
		return RelationConflictPage{}, err
	}
	return browseConflicts(
		ctx,
		request,
		application.Candidates.ListRelationIDs,
		application.RelationConflict,
	)
}

func (application *Application) BrowseClaimConflicts(
	ctx context.Context,
	request ConflictPageRequest,
) (ClaimConflictPage, error) {
	if err := requireConflictApplication(application); err != nil {
		return ClaimConflictPage{}, err
	}
	request, err := normalizeConflictPage(request)
	if err != nil {
		return ClaimConflictPage{}, err
	}
	return browseConflicts(
		ctx,
		request,
		application.Candidates.ListClaimIDs,
		application.ClaimConflict,
	)
}

type conflict interface {
	EntityConflict | RelationConflict | ClaimConflict
}

func browseConflicts[ID knowledge.KnowledgeID, Conflict conflict](
	ctx context.Context,
	request ConflictPageRequest,
	list func(context.Context, ID, int) ([]ID, error),
	read func(context.Context, ID) (Conflict, error),
) (knowledge.Page[Conflict, ID], error) {
	ids, err := list(ctx, ID(request.After), request.Limit+1)
	if err != nil {
		return knowledge.Page[Conflict, ID]{}, err
	}
	page := knowledge.Page[Conflict, ID]{}
	if len(ids) > request.Limit {
		ids = ids[:request.Limit]
		page.HasMore = true
	}
	page.Items = make([]Conflict, len(ids))
	for index, id := range ids {
		page.Items[index], err = read(ctx, id)
		if err != nil {
			return knowledge.Page[Conflict, ID]{}, err
		}
	}
	if len(ids) > 0 {
		page.NextAfter = ids[len(ids)-1]
	}
	return page, nil
}

func normalizeConflictPage(request ConflictPageRequest) (ConflictPageRequest, error) {
	request.After = strings.TrimSpace(request.After)
	if request.Limit == 0 {
		request.Limit = DefaultPageSize
	}
	if request.Limit < 1 || request.Limit > MaximumPageSize {
		return ConflictPageRequest{}, fmt.Errorf(
			"%w: conflict page limit must be between 1 and %d",
			knowledge.ErrInvalidChange,
			MaximumPageSize,
		)
	}
	return request, nil
}

func requireConflictApplication(application *Application) error {
	if application == nil || application.Candidates == nil {
		return errors.New("browse Knowledge conflicts: application is not configured")
	}
	return nil
}
