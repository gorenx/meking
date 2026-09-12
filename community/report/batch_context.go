package report

import (
	"fmt"
	"sort"
	"strconv"
)

type numberedEntity struct {
	row   int
	value Entity
}

type numberedRelation struct {
	row   int
	value Relation
}

type numberedClaim struct {
	row   int
	value Claim
}

// reportBatch owns a disjoint subset of one Community's effective evidence.
// Row numbers always refer to the complete Community input so citations remain
// stable when multiple fragments are later merged into one Report.
type reportBatch struct {
	entities  []numberedEntity
	relations []numberedRelation
	claims    []numberedClaim
}

func numberReportInput(input communityInput) reportBatch {
	result := reportBatch{
		entities:  make([]numberedEntity, len(input.entities)),
		relations: make([]numberedRelation, len(input.relations)),
		claims:    make([]numberedClaim, len(input.claims)),
	}
	for row, value := range input.entities {
		result.entities[row] = numberedEntity{row: row, value: value}
	}
	for row, value := range input.relations {
		result.relations[row] = numberedRelation{row: row, value: value}
	}
	for row, value := range input.claims {
		result.claims[row] = numberedClaim{row: row, value: value}
	}
	return result
}

func renderReportBatch(batch reportBatch, entitiesByID map[string]Entity) (string, error) {
	sections := make([]string, 0, 3)
	if len(batch.entities) > 0 {
		rows := make([][]string, 0, len(batch.entities)+1)
		rows = append(rows, []string{
			"human_readable_id", "title", "description", "degree",
		})
		for _, item := range batch.entities {
			rows = append(rows, []string{
				strconv.Itoa(item.row),
				item.value.Title,
				item.value.Description,
				strconv.Itoa(item.value.Degree),
			})
		}
		data, err := encodeCSV(rows)
		if err != nil {
			return "", fmt.Errorf("encode Report Entities: %w", err)
		}
		sections = append(sections, "-----Entities-----\n"+data)
	}
	if len(batch.claims) > 0 {
		rows := make([][]string, 0, len(batch.claims)+1)
		rows = append(rows, []string{
			"human_readable_id", "subject", "object", "type", "status",
			"start_date", "end_date", "description", "source_text",
		})
		for _, item := range batch.claims {
			claim := item.value
			rows = append(rows, []string{
				strconv.Itoa(item.row),
				claim.SubjectText,
				claim.ObjectText,
				claim.Type,
				claim.Status,
				claim.StartDate,
				claim.EndDate,
				claim.Description,
				claim.SourceText,
			})
		}
		data, err := encodeCSV(rows)
		if err != nil {
			return "", fmt.Errorf("encode Report Claims: %w", err)
		}
		sections = append(sections, "-----Claims-----\n"+data)
	}
	if len(batch.relations) > 0 {
		rows := make([][]string, 0, len(batch.relations)+1)
		rows = append(rows, []string{
			"human_readable_id", "source", "target", "description", "weight",
			"combined_degree",
		})
		for _, item := range batch.relations {
			relation := item.value
			rows = append(rows, []string{
				strconv.Itoa(item.row),
				entitiesByID[relation.SourceEntityID].Title,
				entitiesByID[relation.TargetEntityID].Title,
				relation.Description,
				strconv.FormatFloat(relation.Weight, 'g', -1, 64),
				strconv.Itoa(relation.CombinedDegree),
			})
		}
		data, err := encodeCSV(rows)
		if err != nil {
			return "", fmt.Errorf("encode Report Relations: %w", err)
		}
		sections = append(sections, "-----Relationships-----\n"+data)
	}
	return joinContextSections(sections), nil
}

func mergeReportBatches(left, right reportBatch) reportBatch {
	result := reportBatch{
		entities:  append(append([]numberedEntity(nil), left.entities...), right.entities...),
		relations: append(append([]numberedRelation(nil), left.relations...), right.relations...),
		claims:    append(append([]numberedClaim(nil), left.claims...), right.claims...),
	}
	sort.Slice(result.entities, func(i, j int) bool {
		return result.entities[i].row < result.entities[j].row
	})
	sort.Slice(result.relations, func(i, j int) bool {
		return result.relations[i].row < result.relations[j].row
	})
	sort.Slice(result.claims, func(i, j int) bool {
		return result.claims[i].row < result.claims[j].row
	})
	return result
}

func reportBatchEmpty(batch reportBatch) bool {
	return len(batch.entities) == 0 && len(batch.relations) == 0 && len(batch.claims) == 0
}
