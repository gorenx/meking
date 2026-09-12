package project

import "testing"

func configureTestModel(
	t *testing.T,
	models map[string]ModelConfig,
	modelID string,
	apiKey string,
	baseURL string,
) {
	t.Helper()
	model, found := models[modelID]
	if !found {
		t.Fatalf("model %q is not configured", modelID)
	}
	model.APIKey = apiKey
	model.BaseURL = baseURL
	models[modelID] = model
}
