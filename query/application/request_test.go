package application

import (
	"errors"
	"testing"

	querybase "github.com/memoria-space/meking/query"
)

func TestValidateRequestKeepsMethodSpecificChoicesAtDeliveryBoundary(t *testing.T) {
	level := 2
	valid := []Request{
		{Method: MethodBasic, Question: "basic"},
		{
			Method: MethodLocal, Question: "local",
			Conversation:     []querybase.ConversationTurn{{Role: querybase.RoleUser, Content: "before"}},
			IncludeEntityIDs: []string{"entity-a"}, ExcludeEntityIDs: []string{"entity-b"},
		},
		{
			Method: MethodGlobal, Question: "global", CommunityLevel: &level,
			DynamicCommunitySelection: true,
			Conversation:              []querybase.ConversationTurn{{Role: querybase.RoleAssistant, Content: "before"}},
		},
		{Method: MethodDRIFT, Question: "drift"},
	}
	for _, request := range valid {
		if err := ValidateRequest(request); err != nil {
			t.Fatalf("ValidateRequest(%q) error = %v", request.Method, err)
		}
	}

	invalid := []Request{
		{Method: "unknown", Question: "question"},
		{Method: MethodBasic, Question: " "},
		{Method: MethodBasic, Question: "question", CommunityLevel: &level},
		{Method: MethodLocal, Question: "question", DynamicCommunitySelection: true},
		{
			Method: MethodDRIFT, Question: "question",
			Conversation: []querybase.ConversationTurn{{Role: querybase.RoleUser, Content: "before"}},
		},
		{Method: MethodGlobal, Question: "question", IncludeEntityIDs: []string{"entity"}},
	}
	for _, request := range invalid {
		err := ValidateRequest(request)
		var failure *querybase.Failure
		if !errors.As(err, &failure) || failure.Category != querybase.FailureInvalidInput {
			t.Fatalf("ValidateRequest(%#v) error = %#v", request, err)
		}
	}
}
