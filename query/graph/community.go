package graph

import (
	"context"
	"fmt"
	"strings"
)

func (c *Catalog) BrowseCommunities(ctx context.Context, request CommunityRequest) (CommunityPage, error) {
	request, err := normalizeCommunityRequest(request)
	if err != nil {
		return CommunityPage{}, err
	}
	if c == nil || c.structures == nil {
		return CommunityPage{}, fmt.Errorf("graph Catalog is required")
	}
	structure, err := c.structures.Current(ctx)
	if err != nil {
		return CommunityPage{}, err
	}
	children := make(map[string]int)
	for _, community := range structure.Memberships {
		if community.ParentID != nil {
			children[*community.ParentID]++
		}
	}
	if request.ParentID != nil {
		found := false
		for _, community := range structure.Memberships {
			if community.ID == *request.ParentID {
				found = true
				break
			}
		}
		if !found {
			return CommunityPage{}, ErrCommunityNotFound
		}
	}
	matched := make([]Community, 0)
	needle := strings.ToLower(request.Query)
	for _, community := range structure.Memberships {
		if request.Query != "" {
			if !strings.Contains(strings.ToLower(community.ID), needle) {
				continue
			}
		} else if request.ParentID == nil {
			if community.ParentID != nil {
				continue
			}
		} else if community.ParentID == nil || *community.ParentID != *request.ParentID {
			continue
		}
		matched = append(matched, Community{
			ID: community.ID, Number: community.Number, Level: community.Level,
			ParentID: copyString(community.ParentID), ChildCount: children[community.ID],
			EntityCount: len(community.EntityIDs),
		})
	}
	start := (request.Page - 1) * request.PageSize
	page := make([]Community, 0)
	if start < len(matched) {
		end := min(start+request.PageSize, len(matched))
		page = append(page, matched[start:end]...)
	}
	return CommunityPage{
		StructureID: structure.ID, CommunitySetID: structure.CommunitySetID,
		CorporaID: structure.CorporaID, Page: request.Page, PageSize: request.PageSize,
		Total: len(matched), Communities: page,
	}, nil
}

func normalizeCommunityRequest(request CommunityRequest) (CommunityRequest, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.ParentID != nil {
		value := strings.TrimSpace(*request.ParentID)
		if value == "" {
			return CommunityRequest{}, fmt.Errorf("%w: parent Community ID is empty", ErrInvalidRequest)
		}
		request.ParentID = &value
	}
	if request.Query != "" && request.ParentID != nil {
		return CommunityRequest{}, fmt.Errorf("%w: query and parent ID cannot be combined", ErrInvalidRequest)
	}
	if request.Page == 0 {
		request.Page = 1
	}
	if request.PageSize == 0 {
		request.PageSize = DefaultCommunitySize
	}
	if request.Page < 1 || request.PageSize < 1 || request.PageSize > MaximumCommunitySize {
		return CommunityRequest{}, fmt.Errorf(
			"%w: page must be positive and page size must be between 1 and %d",
			ErrInvalidRequest, MaximumCommunitySize,
		)
	}
	maximumInt := int(^uint(0) >> 1)
	if request.Page-1 > maximumInt/request.PageSize {
		return CommunityRequest{}, fmt.Errorf("%w: Community page is too large", ErrInvalidRequest)
	}
	return request, nil
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
