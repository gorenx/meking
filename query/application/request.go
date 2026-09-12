package application

import (
	"fmt"
	"strings"

	querybase "github.com/memoria-space/meking/query"
)

func ValidateRequest(request Request) error {
	switch request.Method {
	case MethodBasic, MethodLocal, MethodGlobal, MethodDRIFT:
	default:
		return querybase.NewInvalidInputFailure(
			fmt.Sprintf("unsupported query method %q", request.Method),
			nil,
		)
	}
	if strings.TrimSpace(request.Question) == "" {
		return querybase.NewInvalidInputFailure("question must not be empty", nil)
	}
	if request.CommunityLevel != nil {
		if request.Method != MethodGlobal {
			return querybase.NewInvalidInputFailure(
				"community level is available only for Global Search",
				nil,
			)
		}
		if *request.CommunityLevel < 0 {
			return querybase.NewInvalidInputFailure("community level must be non-negative", nil)
		}
	}
	if request.DynamicCommunitySelection && request.Method != MethodGlobal {
		return querybase.NewInvalidInputFailure(
			"dynamic community selection is available only for Global Search",
			nil,
		)
	}
	if len(request.Conversation) > 0 && request.Method != MethodLocal && request.Method != MethodGlobal {
		return querybase.NewInvalidInputFailure(
			"conversation is available only for Local and Global Search",
			nil,
		)
	}
	for index, turn := range request.Conversation {
		switch turn.Role {
		case querybase.RoleSystem, querybase.RoleUser, querybase.RoleAssistant:
		default:
			return querybase.NewInvalidInputFailure(
				fmt.Sprintf("conversation turn %d has an unsupported role", index),
				nil,
			)
		}
		if strings.TrimSpace(turn.Content) == "" {
			return querybase.NewInvalidInputFailure(
				fmt.Sprintf("conversation turn %d has empty content", index),
				nil,
			)
		}
	}
	if (len(request.IncludeEntityIDs) > 0 || len(request.ExcludeEntityIDs) > 0) &&
		request.Method != MethodLocal {
		return querybase.NewInvalidInputFailure(
			"Entity filters are available only for Local Search",
			nil,
		)
	}
	return nil
}
