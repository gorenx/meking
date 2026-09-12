package knowledge

import "testing"

func TestSameSubjectIncludesKnowledgeKind(t *testing.T) {
	const id = "10000000-0000-4000-8000-000000000001"
	entity := EntityID(id)
	relation := RelationID(id)

	if !SameSubject(entity, entity) {
		t.Fatal("same Entity Subject was not equal")
	}
	if !SameSubject(relation, relation) {
		t.Fatal("same Relation Subject was not equal")
	}
	if SameSubject(entity, relation) || SameSubject(relation, entity) {
		t.Fatal("Subjects with different Knowledge kinds were equal")
	}
}
