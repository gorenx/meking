package controlplane

import (
	"fmt"
	"slices"
)

type Action string

const (
	ConvertDocument          Action = "convert_document"
	CreateTextUnits          Action = "create_text_units"
	ExtractKnowledge         Action = "extract_knowledge"
	IndexEntityVectors       Action = "index_entity_vectors"
	DeriveCommunityStructure Action = "derive_community_structure"
	PublishEpoch             Action = "publish_epoch"
	GenerateCommunityReports Action = "generate_community_reports"
)

var actions = []Action{
	ConvertDocument,
	CreateTextUnits,
	ExtractKnowledge,
	IndexEntityVectors,
	DeriveCommunityStructure,
	PublishEpoch,
	GenerateCommunityReports,
}

func Actions() []Action {
	return append([]Action(nil), actions...)
}

func ParseAction(value string) (Action, error) {
	action := Action(value)
	if !slices.Contains(actions, action) {
		return "", fmt.Errorf("%w: unsupported Action %q", ErrInvalidAction, value)
	}
	return action, nil
}
