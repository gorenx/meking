// Package prompt renders Query-owned templates without exposing a standalone
// application service. Query aggregates remain responsible for allowed fields.
package prompt

import (
	"errors"
	"fmt"
	"strings"
)

// Render substitutes the exact named fields supplied by an owning Query
// aggregate. Double braces escape literal braces; unknown or unmatched fields
// fail before a model request is made.
func Render(template string, fields map[string]string) (string, error) {
	var rendered strings.Builder
	rendered.Grow(len(template))
	for index := 0; index < len(template); {
		switch template[index] {
		case '{':
			if index+1 < len(template) && template[index+1] == '{' {
				rendered.WriteByte('{')
				index += 2
				continue
			}
			end := strings.IndexByte(template[index+1:], '}')
			if end < 0 {
				return "", errors.New("unmatched '{' in prompt")
			}
			end += index + 1
			field := template[index+1 : end]
			value, exists := fields[field]
			if !exists {
				return "", fmt.Errorf("unknown prompt field %q", field)
			}
			rendered.WriteString(value)
			index = end + 1
		case '}':
			if index+1 < len(template) && template[index+1] == '}' {
				rendered.WriteByte('}')
				index += 2
				continue
			}
			return "", errors.New("unmatched '}' in prompt")
		default:
			rendered.WriteByte(template[index])
			index++
		}
	}
	return rendered.String(), nil
}
