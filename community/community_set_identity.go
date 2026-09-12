package community

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/memoria-space/meking/internal/uuid"
)

// ValidateCommunitySetID verifies the canonical lowercase UUID v4 used to
// identify one immutable complete Community hierarchy.
func ValidateCommunitySetID(id CommunitySetID) error {
	if !uuid.IsCanonicalV4(string(id)) {
		return fmt.Errorf(
			"%w: CommunitySet ID %q is not a canonical lowercase UUID v4",
			ErrInvalidCommunitySet,
			id,
		)
	}
	return nil
}

// ValidateCommunityID verifies the lowercase SHA-256 representation derived
// from one Community's complete sorted EntityID membership.
func ValidateCommunityID(id CommunityID) error {
	if len(id) != sha256.Size*2 {
		return fmt.Errorf(
			"%w: Community ID %q is not a lowercase SHA-256 digest",
			ErrInvalidCommunitySet,
			id,
		)
	}
	for _, character := range []byte(id) {
		if !isCommunityLowerHex(character) {
			return fmt.Errorf(
				"%w: Community ID %q is not a lowercase SHA-256 digest",
				ErrInvalidCommunitySet,
				id,
			)
		}
	}
	return nil
}

func newCommunitySetID() (CommunitySetID, error) {
	value, err := uuid.NewV4()
	return CommunitySetID(value), err
}

// communityID hashes a non-empty, strictly sorted EntityID set. Each member is
// encoded as an unsigned 64-bit big-endian byte length followed by the UTF-8
// bytes of its canonical lowercase UUID string, so boundaries cannot collide.
func communityID(entityIDs []string) CommunityID {
	hash := sha256.New()
	var length [8]byte
	for _, entityID := range entityIDs {
		binary.BigEndian.PutUint64(length[:], uint64(len(entityID)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(entityID))
	}
	return CommunityID(hex.EncodeToString(hash.Sum(nil)))
}

func isCommunityLowerHex(character byte) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f'
}
