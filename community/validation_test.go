package community

import (
	"errors"
	"testing"
)

func TestHierarchyValidateRejectsMissingParentAndMembershipEscape(t *testing.T) {
	tests := []struct {
		name      string
		hierarchy Hierarchy
	}{
		{
			name: "missing parent",
			hierarchy: Hierarchy{Communities: []Community{
				{ID: 0, Level: 0, ParentID: -1, Nodes: []string{"A"}, Final: false},
				{ID: 1, Level: 1, ParentID: 99, Nodes: []string{"A"}, Final: true},
			}},
		},
		{
			name: "node outside parent",
			hierarchy: Hierarchy{Communities: []Community{
				{ID: 0, Level: 0, ParentID: -1, Nodes: []string{"A"}, Final: false},
				{ID: 1, Level: 1, ParentID: 0, Nodes: []string{"B"}, Final: true},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.hierarchy.Validate(); !errors.Is(err, ErrInvalidHierarchy) {
				t.Fatalf("Validate() error = %v, want ErrInvalidHierarchy", err)
			}
		})
	}
}
