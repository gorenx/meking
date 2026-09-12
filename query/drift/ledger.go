package drift

import querybase "github.com/memoria-space/meking/query"

type citationLedger struct {
	next    map[querybase.CitationDataset]int
	records []querybase.CitationRecord
}

func newCitationLedger() *citationLedger {
	return &citationLedger{next: make(map[querybase.CitationDataset]int)}
}

func (l *citationLedger) clone() *citationLedger {
	result := newCitationLedger()
	for dataset, next := range l.next {
		result.next[dataset] = next
	}
	result.records = copyCitationRecords(l.records)
	return result
}

func (l *citationLedger) bind(record querybase.CitationRecord) querybase.CitationReference {
	reference := querybase.CitationReference{
		Dataset:  record.Reference.Dataset,
		RecordID: l.next[record.Reference.Dataset],
	}
	l.next[reference.Dataset]++
	record.Reference = reference
	record.TextUnitIDs = append([]string(nil), record.TextUnitIDs...)
	l.records = append(l.records, record)
	return reference
}

func copyCitationRecords(values []querybase.CitationRecord) []querybase.CitationRecord {
	result := make([]querybase.CitationRecord, len(values))
	for index, value := range values {
		value.TextUnitIDs = append([]string(nil), value.TextUnitIDs...)
		result[index] = value
	}
	return result
}
