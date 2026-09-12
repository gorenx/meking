package knowledge

import "slices"

// Equal reports whether both values are the same Entity, including its stable
// identity and complete formal content.
func (entity Entity) Equal(other Entity) bool {
	return entity.ID == other.ID &&
		entity.Title == other.Title &&
		entity.Type == other.Type &&
		entity.Description == other.Description &&
		slices.Equal(entity.Aliases, other.Aliases)
}

// SameSubject compares both the Subject kind and its persistent identity.
func SameSubject(left Subject, right Subject) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	switch left := left.(type) {
	case EntityID:
		right, ok := right.(EntityID)
		return ok && left == right
	case RelationID:
		right, ok := right.(RelationID)
		return ok && left == right
	default:
		return false
	}
}

// Equal reports whether both values are the same Claim, including its stable
// identity and complete formal content.
func (claim Claim) Equal(other Claim) bool {
	return claim.ID == other.ID &&
		claim.Type == other.Type &&
		claim.Description == other.Description &&
		SameSubject(claim.Subject, other.Subject)
}
