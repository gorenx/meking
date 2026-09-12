package extraction

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const graphExtractionSchemaName = "knowledge_graph"

var graphExtractionSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "entities": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "name": {"type": "string", "minLength": 1},
          "type": {"type": "string", "minLength": 1},
          "aliases": {"type": "array", "items": {"type": "string"}},
          "description": {"type": "string", "minLength": 1}
        },
        "required": ["name", "type", "aliases", "description"]
      }
    },
    "relations": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "source": {"type": "string", "minLength": 1},
          "target": {"type": "string", "minLength": 1},
          "type": {"type": "string", "minLength": 1},
          "description": {"type": "string", "minLength": 1},
          "weight": {"type": "number", "minimum": 0}
        },
        "required": ["source", "target", "type", "description", "weight"]
      }
    }
  },
  "required": ["entities", "relations"]
}`)

type graphExtractionResult struct {
	Entities  []graphEntityResult   `json:"entities"`
	Relations []graphRelationResult `json:"relations"`
}

type graphEntityResult struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
}

type graphRelationResult struct {
	Source      string   `json:"source"`
	Target      string   `json:"target"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Weight      *float64 `json:"weight"`
}

// parseGraphExtraction validates every Agent result before converting it into
// evidence observations. Multiple JSON objects are accepted because each
// gleaning round returns one independently schema-constrained result.
func parseGraphExtraction(result, textUnitID string) (graphObservations, error) {
	observations := emptyGraphObservations()
	decoder := json.NewDecoder(strings.NewReader(result))
	decoder.DisallowUnknownFields()
	resultCount := 0
	for {
		var extracted graphExtractionResult
		if err := decoder.Decode(&extracted); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			reason := fmt.Sprintf("result must be valid graph JSON: %v", err)
			return graphObservations{}, rejectResult(result, reason, fmt.Errorf("%w: %s", ErrInvalidGraphExtraction, reason))
		}
		resultCount++
		if extracted.Entities == nil || extracted.Relations == nil {
			reason := fmt.Sprintf("result %d must contain Entity and Relation arrays", resultCount-1)
			return graphObservations{}, rejectResult(result, reason, fmt.Errorf("%w: %s", ErrInvalidGraphExtraction, reason))
		}
		for index, entity := range extracted.Entities {
			observation, err := parseEntityResult(entity, textUnitID)
			if err != nil {
				reason := fmt.Sprintf("invalid Entity at result %d index %d: %v", resultCount-1, index, err)
				return graphObservations{}, rejectResult(marshalRejectedPart(entity), reason, fmt.Errorf("%w: %s", ErrInvalidGraphExtraction, reason))
			}
			observations.Entities = append(observations.Entities, observation)
		}
		for index, relation := range extracted.Relations {
			observation, err := parseRelationResult(relation, textUnitID)
			if err != nil {
				reason := fmt.Sprintf("invalid Relation at result %d index %d: %v", resultCount-1, index, err)
				return graphObservations{}, rejectResult(marshalRejectedPart(relation), reason, fmt.Errorf("%w: %s", ErrInvalidGraphExtraction, reason))
			}
			observations.Relationships = append(observations.Relationships, observation)
		}
	}
	if resultCount == 0 {
		reason := "result must contain one graph JSON object"
		return graphObservations{}, rejectResult(result, reason, fmt.Errorf("%w: %s", ErrInvalidGraphExtraction, reason))
	}
	return observations, nil
}

func parseEntityResult(result graphEntityResult, textUnitID string) (entityObservation, error) {
	title := cleanGraphField(upperGraphField(result.Name))
	typeName := cleanGraphField(upperGraphField(result.Type))
	description := cleanGraphField(result.Description)
	aliases, valid := normalizeEntityAliases(result.Aliases, title)
	if title == "" || typeName == "" || description == "" || !valid {
		return entityObservation{}, errors.New("name, type, aliases, and description are required")
	}
	return entityObservation{
		Title:       title,
		Type:        typeName,
		Aliases:     aliases,
		Description: description,
		TextUnitID:  textUnitID,
	}, nil
}

func parseRelationResult(result graphRelationResult, textUnitID string) (relationshipObservation, error) {
	source := cleanGraphField(upperGraphField(result.Source))
	target := cleanGraphField(upperGraphField(result.Target))
	typeName := cleanGraphField(upperGraphField(result.Type))
	description := cleanGraphField(result.Description)
	if source == "" || target == "" || typeName == "" || description == "" || result.Weight == nil ||
		math.IsNaN(*result.Weight) || math.IsInf(*result.Weight, 0) || *result.Weight < 0 {
		return relationshipObservation{}, errors.New("source, target, type, description, and a non-negative finite weight are required")
	}
	return relationshipObservation{
		Source:      source,
		Target:      target,
		Type:        typeName,
		Description: description,
		TextUnitID:  textUnitID,
		Weight:      *result.Weight,
	}, nil
}

func normalizeEntityAliases(aliases []string, title string) ([]string, bool) {
	if aliases == nil {
		return nil, false
	}
	normalized := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		alias = cleanGraphField(upperGraphField(alias))
		if alias == "" || alias == title {
			continue
		}
		normalized = append(normalized, alias)
	}
	sort.Strings(normalized)
	result := normalized[:0]
	for _, alias := range normalized {
		if len(result) == 0 || result[len(result)-1] != alias {
			result = append(result, alias)
		}
	}
	if result == nil {
		return []string{}, true
	}
	return result, true
}

func marshalRejectedPart(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func upperGraphField(value string) string {
	return cases.Upper(language.Und).String(value)
}

func cleanGraphField(value string) string {
	value = html.UnescapeString(strings.TrimSpace(value))
	return strings.Map(func(r rune) rune {
		if r <= 0x1f || r >= 0x7f && r <= 0x9f {
			return -1
		}
		return r
	}, value)
}
