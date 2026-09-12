// Package uuid generates and recognizes the UUID representation shared by
// immutable and versioned identities across the repository. It deliberately
// does not decide when an identity is allocated or which domain error an
// invalid identity produces; those rules remain with the owning domain.
package uuid

import "crypto/rand"

// NewV4 returns a randomly generated UUID v4 in canonical lowercase form.
func NewV4() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	const hexadecimal = "0123456789abcdef"
	var value [36]byte
	rawIndex := 0
	for index := range value {
		switch index {
		case 8, 13, 18, 23:
			value[index] = '-'
		default:
			part := raw[rawIndex/2]
			if rawIndex%2 == 0 {
				value[index] = hexadecimal[part>>4]
			} else {
				value[index] = hexadecimal[part&0x0f]
			}
			rawIndex++
		}
	}
	return string(value[:]), nil
}

// IsCanonicalV4 reports whether value is a lowercase UUID v4 with the
// canonical 8-4-4-4-12 representation and an RFC 4122 variant.
func IsCanonicalV4(value string) bool {
	if len(value) != 36 || value[14] != '4' {
		return false
	}
	for index, character := range []byte(value) {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !isLowerHex(character) {
				return false
			}
		}
	}
	switch value[19] {
	case '8', '9', 'a', 'b':
		return true
	default:
		return false
	}
}

func isLowerHex(character byte) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f'
}
