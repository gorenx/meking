package extraction

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const claimFixtureResponse = `(COMPANY A<|>GOVERNMENT AGENCY B<|>ANTI-COMPETITIVE PRACTICES<|>TRUE<|>2022-01-10T00:00:00<|>2022-01-10T00:00:00<|>Company A was fined for bid rigging<|>According to the article, Company A was fined.)<|COMPLETE|>`

func TestParseClaimExtractionPreservesFieldsAndSource(t *testing.T) {
	claims, err := parseClaimExtraction(claimFixtureResponse, "tu-1")
	if err != nil {
		t.Fatalf("parseClaimExtraction() error = %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("parseClaimExtraction() = %#v, want one Claim", claims)
	}
	got := claims[0]
	if got.Subject != "COMPANY A" || got.Object != "GOVERNMENT AGENCY B" ||
		got.Type != "ANTI-COMPETITIVE PRACTICES" || got.Status != "TRUE" ||
		got.StartDate != "2022-01-10T00:00:00" || got.EndDate != "2022-01-10T00:00:00" ||
		got.Description != "Company A was fined for bid rigging" ||
		got.SourceText != "According to the article, Company A was fined." || got.TextUnitID != "tu-1" {
		t.Fatalf("Claim = %#v", got)
	}
}

func TestParseClaimExtractionRejectsIncompleteEvidence(t *testing.T) {
	_, err := parseClaimExtraction("##(A<|>B)##(<|>B<|>TYPE<|>TRUE<|>NONE<|>NONE<|>description<|>source)", "tu")
	if !errors.Is(err, ErrInvalidClaimExtraction) {
		t.Fatalf("parseClaimExtraction() error = %v, want ErrInvalidClaimExtraction", err)
	}
}

func TestParseClaimExtractionPreservesQuotedSourceText(t *testing.T) {
	claims, err := parseClaimExtraction(
		"(A<|>B<|>TYPE<|>TRUE<|>NONE<|>NONE<|>description<|>&lt;quoted&gt; source)",
		"tu",
	)
	if err != nil {
		t.Fatalf("parseClaimExtraction() error = %v", err)
	}
	if len(claims) != 1 || claims[0].SourceText != "&lt;quoted&gt; source" {
		t.Fatalf("claims = %#v, want source text preserved", claims)
	}
}

func TestClaimExtractorRestoresSourceOrder(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: claimFixtureResponse},
		{Content: `(PERSON C<|>NONE<|>CORRUPTION<|>SUSPECTED<|>2015-01-01<|>2015-12-31<|>Person C was suspected<|>Person C was suspected.)`},
	}}
	extractor, err := newclaimExtractor(model, ClaimExtractionConfig{
		Prompt:      "entities={entity_specs}; description={claim_description}; text={input_text}",
		EntityTypes: []string{"organization", "person"}, Description: "relevant facts",
	})
	if err != nil {
		t.Fatalf("newclaimExtractor() error = %v", err)
	}
	claims, err := extractor.extractClaims(context.Background(), []TextUnitInput{
		{ID: "tu-1", Text: " first "}, {ID: "tu-2", Text: "second"},
	})
	if err != nil {
		t.Fatalf("extractClaims() error = %v", err)
	}
	if len(claims) != 2 || claims[0].TextUnitID != "tu-1" || claims[1].TextUnitID != "tu-2" {
		t.Fatalf("claims = %#v", claims)
	}
	if got := model.requests[0].Messages[0].Content; got != "entities=['organization', 'person']; description=relevant facts; text=first" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestClaimExtractorRetainsGleanedClaimsAndConversation(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: claimFixtureResponse},
		{Content: `(PERSON C<|>NONE<|>CORRUPTION<|>TRUE<|>NONE<|>NONE<|>gleaned<|>gleaned source)<|COMPLETE|>`},
		{Content: "Y"},
		{Content: `(PERSON D<|>NONE<|>FRAUD<|>TRUE<|>NONE<|>NONE<|>gleaned again<|>source)`},
	}}
	extractor, err := newclaimExtractor(model, ClaimExtractionConfig{
		Prompt: "{input_text}", MaxGleanings: 2,
	})
	if err != nil {
		t.Fatalf("newclaimExtractor() error = %v", err)
	}
	claims, err := extractor.extractClaims(context.Background(), []TextUnitInput{{ID: "tu", Text: "text"}})
	if err != nil {
		t.Fatalf("extractClaims() error = %v", err)
	}
	if len(claims) != 3 || claims[0].Subject != "COMPANY A" ||
		claims[1].Subject != "PERSON C" || claims[2].Subject != "PERSON D" {
		t.Fatalf("claims = %#v, want initial and both gleaning responses", claims)
	}
	wantRoles := [][]CompletionRole{
		{CompletionRoleUser},
		{CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser},
		{CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser},
		{CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser, CompletionRoleAssistant, CompletionRoleUser},
	}
	for index, request := range model.requests {
		roles := make([]CompletionRole, len(request.Messages))
		for messageIndex, message := range request.Messages {
			roles[messageIndex] = message.Role
		}
		if !reflect.DeepEqual(roles, wantRoles[index]) {
			t.Fatalf("request %d roles = %v, want %v", index, roles, wantRoles[index])
		}
	}
}

func TestClaimExtractorRejectsInvalidContinuationDecision(t *testing.T) {
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: claimFixtureResponse},
		{Content: claimFixtureResponse},
		{Content: "maybe"},
		{Content: "still maybe"},
	}}
	extractor, err := newclaimExtractor(model, ClaimExtractionConfig{
		Prompt:       "{input_text}",
		MaxGleanings: 2,
	})
	if err != nil {
		t.Fatalf("newclaimExtractor() error = %v", err)
	}

	_, err = extractor.processTextUnit(t.Context(), "text")
	if !errors.Is(err, ErrInvalidClaimExtraction) {
		t.Fatalf("processTextUnit() error = %v, want ErrInvalidClaimExtraction", err)
	}
}

func TestClaimExtractorReturnsRejectedRecordAndReasonForCorrection(t *testing.T) {
	invalid := `(A<|>B<|>TYPE)`
	model := &scriptedCompletionModel{responses: []CompletionResponse{
		{Content: invalid},
		{Content: claimFixtureResponse},
	}}
	extractor, err := newclaimExtractor(model, ClaimExtractionConfig{Prompt: "{input_text}"})
	if err != nil {
		t.Fatalf("newclaimExtractor() error = %v", err)
	}

	claims, err := extractor.extractClaims(t.Context(), []TextUnitInput{{ID: "tu", Text: "text"}})
	if err != nil {
		t.Fatalf("extractClaims() error = %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("claims = %#v", claims)
	}
	if len(model.requests) != 2 {
		t.Fatalf("completion calls = %d, want initial call and one correction", len(model.requests))
	}
	correction := model.requests[1].Messages
	if len(correction) != 3 || correction[1].Content != invalid {
		t.Fatalf("correction conversation = %#v", correction)
	}
	if !strings.Contains(correction[2].Content, invalid) ||
		!strings.Contains(correction[2].Content, "got 3 fields, want 8") {
		t.Fatalf("correction prompt = %q", correction[2].Content)
	}
}

func TestClaimExtractorRejectsUnitFailureAndPropagatesCancellation(t *testing.T) {
	expected := errors.New("provider failed")
	model := &scriptedCompletionModel{errors: []error{expected}}
	extractor, err := newclaimExtractor(model, ClaimExtractionConfig{Prompt: "{input_text}"})
	if err != nil {
		t.Fatalf("newclaimExtractor() error = %v", err)
	}
	claims, err := extractor.extractClaims(context.Background(), []TextUnitInput{{ID: "tu", Text: "text"}})
	if !errors.Is(err, expected) || len(claims) != 0 || !strings.Contains(err.Error(), `"tu"`) {
		t.Fatalf("claims=%#v error=%v", claims, err)
	}

	cancelled, err := newclaimExtractor(
		&scriptedCompletionModel{errors: []error{context.Canceled}},
		ClaimExtractionConfig{Prompt: "{input_text}"},
	)
	if err != nil {
		t.Fatalf("newclaimExtractor() error = %v", err)
	}
	if _, err := cancelled.extractClaims(context.Background(), []TextUnitInput{{ID: "tu", Text: "text"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("extractClaims() error = %v, want context.Canceled", err)
	}
}

func TestClaimExtractorRejectsIncompleteAuthoritativeBatch(t *testing.T) {
	firstFailure := errors.New("first provider failure")
	secondFailure := errors.New("second provider failure")
	model := &scriptedCompletionModel{
		errors: []error{firstFailure, secondFailure},
	}
	extractor, err := newclaimExtractor(model, ClaimExtractionConfig{
		Prompt: "{input_text}", MaxConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("newclaimExtractor() error = %v", err)
	}

	_, err = extractor.extractClaims(context.Background(), []TextUnitInput{
		{ID: "tu-1", Text: "first"},
		{ID: "tu-2", Text: "second"},
	})
	if err == nil {
		t.Fatal("extractClaims() error = nil")
	}
	firstIndex := strings.Index(err.Error(), `"tu-1"`)
	secondIndex := strings.Index(err.Error(), `"tu-2"`)
	if !errors.Is(err, firstFailure) || !errors.Is(err, secondFailure) ||
		firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("extractClaims() error = %v, want both failures in TextUnit order", err)
	}
}
