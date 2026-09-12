package knowledge

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"
)

// HashEntity hashes only the canonical content that can change inside one
// stable Entity identity. Source Metadata and graph-derived values are not
// formal content.
func HashEntity(entity Entity) (Hash, error) {
	if err := ValidateEntity(entity); err != nil {
		return Hash{}, err
	}
	var canonical bytes.Buffer
	writeStrings(&canonical, entity.Aliases)
	writeString(&canonical, entity.Description)
	return sha256.Sum256(canonical.Bytes()), nil
}

// HashRelation hashes only the description inside one stable
// SourceEntityID+TargetEntityID+Type identity.
func HashRelation(relation Relation) (Hash, error) {
	if err := ValidateRelation(relation); err != nil {
		return Hash{}, err
	}
	var canonical bytes.Buffer
	writeString(&canonical, relation.Description)
	return sha256.Sum256(canonical.Bytes()), nil
}

// HashClaim hashes only the description inside one stable Subject+Type
// identity. Parser fields and Evidence remain Source Metadata.
func HashClaim(claim Claim) (Hash, error) {
	if err := ValidateClaim(claim); err != nil {
		return Hash{}, err
	}
	var canonical bytes.Buffer
	writeString(&canonical, claim.Description)
	return sha256.Sum256(canonical.Bytes()), nil
}

func writeString(target *bytes.Buffer, value string) {
	writeUint64(target, uint64(len(value)))
	_, _ = target.WriteString(value)
}

func writeStrings(target *bytes.Buffer, values []string) {
	writeUint64(target, uint64(len(values)))
	for _, value := range values {
		writeString(target, value)
	}
}

func writeUint64(target *bytes.Buffer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = target.Write(encoded[:])
}

func normalizedStrings(values []string) []string {
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	result := normalized[:0]
	for _, value := range normalized {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
