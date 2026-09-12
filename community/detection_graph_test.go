package community

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestDetectionGraphValidatesOneKnowledgeSource(t *testing.T) {
	valid := Graph{
		Entities: []EntityReference{
			{ID: setEntityA, Version: 1},
			{ID: setEntityB, Version: 2},
		},
		Relations: []GraphRelation{{
			Reference:      RelationReference{ID: setRelation, Version: 3},
			SourceEntityID: setEntityA,
			TargetEntityID: setEntityB,
			Weight:         2,
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if weighted := valid.weightedEdges(); len(weighted) != 1 ||
		weighted[0].Source != setEntityA ||
		weighted[0].Target != setEntityB ||
		weighted[0].Weight != 2 {
		t.Fatalf("weightedEdges() = %#v", weighted)
	}

	tests := map[string]func(*Graph){
		"unsorted entities": func(graph *Graph) {
			graph.Entities[0], graph.Entities[1] = graph.Entities[1], graph.Entities[0]
		},
		"missing source": func(graph *Graph) {
			graph.Relations[0].SourceEntityID = setEntityC
		},
		"invalid relation version": func(graph *Graph) {
			graph.Relations[0].Reference.Version = 0
		},
		"negative weight": func(graph *Graph) {
			graph.Relations[0].Weight = -1
		},
		"non-finite weight": func(graph *Graph) {
			graph.Relations[0].Weight = math.Inf(1)
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			graph := Graph{
				Entities:  append([]EntityReference(nil), valid.Entities...),
				Relations: append([]GraphRelation(nil), valid.Relations...),
			}
			change(&graph)
			err := graph.Validate()
			if !errors.Is(err, ErrInvalidDetectionGraph) {
				t.Fatalf("Validate() error = %v, want ErrInvalidDetectionGraph", err)
			}
		})
	}
}

func TestDetectRejectsInvalidGraph(t *testing.T) {
	graph := Graph{
		Entities: []EntityReference{{ID: setEntityA, Version: 1}},
		Relations: []GraphRelation{{
			Reference:      RelationReference{ID: setRelation, Version: 1},
			SourceEntityID: setEntityA,
			TargetEntityID: setEntityB,
			Weight:         1,
		}},
	}
	_, err := Detect(
		t.Context(),
		graph,
		DefaultDetectConfig(),
	)
	if !errors.Is(err, ErrInvalidDetectionGraph) ||
		!strings.Contains(err.Error(), setEntityB) {
		t.Fatalf("Detect() error = %v", err)
	}
}
