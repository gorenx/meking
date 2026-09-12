package extraction

import (
	"errors"
	"fmt"
	"strings"
)

const (
	claimTupleDelimiter      = "<|>"
	claimRecordDelimiter     = "##"
	claimCompletionDelimiter = "<|COMPLETE|>"
)

var ErrInvalidClaimExtraction = errors.New("invalid Claim extraction protocol")

// parseClaimExtraction converts delimiter-formatted model output into complete
// source-linked assertions. Malformed records reject the complete TextUnit.
func parseClaimExtraction(result, textUnitID string) ([]claimObservation, error) {
	claims := make([]claimObservation, 0)
	result = strings.TrimSpace(result)
	result = strings.TrimSpace(strings.TrimSuffix(result, claimCompletionDelimiter))
	for recordIndex, rawRecord := range strings.Split(result, claimRecordDelimiter) {
		rawRecord = strings.TrimSpace(rawRecord)
		if rawRecord == "" {
			continue
		}
		if !strings.HasPrefix(rawRecord, "(") || !strings.HasSuffix(rawRecord, ")") {
			reason := fmt.Sprintf("record %d must be enclosed in parentheses", recordIndex)
			return nil, rejectResult(rawRecord, reason, fmt.Errorf("%w: %s", ErrInvalidClaimExtraction, reason))
		}
		record := rawRecord[1 : len(rawRecord)-1]
		fields := strings.Split(record, claimTupleDelimiter)
		if len(fields) != 8 {
			reason := fmt.Sprintf("invalid field count at record %d: got %d fields, want 8", recordIndex, len(fields))
			return nil, rejectResult(rawRecord, reason, fmt.Errorf("%w: %s", ErrInvalidClaimExtraction, reason))
		}
		subject := strings.TrimSpace(fields[0])
		object := strings.TrimSpace(fields[1])
		typeName := strings.TrimSpace(fields[2])
		status := strings.TrimSpace(fields[3])
		startDate := strings.TrimSpace(fields[4])
		endDate := strings.TrimSpace(fields[5])
		description := strings.TrimSpace(fields[6])
		sourceText := strings.TrimSpace(fields[7])
		if subject == "" || object == "" || typeName == "" || status == "" ||
			startDate == "" || endDate == "" || description == "" || sourceText == "" ||
			strings.TrimSpace(textUnitID) == "" {
			reason := fmt.Sprintf("empty required field at record %d", recordIndex)
			return nil, rejectResult(rawRecord, reason, fmt.Errorf("%w: %s", ErrInvalidClaimExtraction, reason))
		}
		claims = append(claims, claimObservation{
			Subject: subject, Object: object,
			Type: typeName, Status: status,
			StartDate: startDate, EndDate: endDate,
			Description: description, SourceText: sourceText,
			TextUnitID: textUnitID,
		})
	}
	return claims, nil
}
