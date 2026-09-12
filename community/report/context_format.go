package report

import (
	"bytes"
	"encoding/csv"
)

func encodeCSV(rows [][]string) (string, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(rows); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func joinContextSections(sections []string) string {
	if len(sections) == 0 {
		return ""
	}
	result := sections[0]
	for _, section := range sections[1:] {
		result += "\n\n" + section
	}
	return result
}
