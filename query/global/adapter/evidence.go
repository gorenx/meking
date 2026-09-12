package adapter

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	querybase "github.com/memoria-space/meking/query"
	queryglobal "github.com/memoria-space/meking/query/global"
	queryreport "github.com/memoria-space/meking/query/report"
)

// KnowledgeReads is the Knowledge application capability needed to resolve
// immutable Entity versions after Global has fixed one Epoch.
type KnowledgeReads interface {
	OpenCurrent(context.Context) (knowledge.View, error)
}

type Metadata interface {
	Entity(context.Context, knowledge.Reference[knowledge.EntityID]) (provenance.EntityMetadata, error)
}

// EvidenceReader combines a ReportSet-fixed Query view with the exact Entity
// evidence used for Global Search occurrence weights. It does not select
// reports or reinterpret current Knowledge.
type EvidenceReader struct {
	reports   queryglobal.ReportReader
	knowledge KnowledgeReads
	metadata  Metadata
}

var _ queryglobal.ReportEvidenceReader = (*EvidenceReader)(nil)

// NewEvidenceReader creates the Global Search evidence adapter from Query
// Report and Knowledge application APIs.
func NewEvidenceReader(
	reports queryglobal.ReportReader,
	knowledge KnowledgeReads,
	metadata Metadata,
) (*EvidenceReader, error) {
	if reports == nil {
		return nil, errors.New("create Global evidence reader: Report reader is required")
	}
	if knowledge == nil {
		return nil, errors.New("create Global evidence reader: Knowledge reader is required")
	}
	if metadata == nil {
		return nil, errors.New("create Global evidence reader: Metadata is required")
	}
	return &EvidenceReader{
		reports:   reports,
		knowledge: knowledge,
		metadata:  metadata,
	}, nil
}

// ReportEvidence reads the exact ReportSet named by selected and every distinct
// exact Entity version referenced by its Reports. Historical Knowledge rows are
// immutable, so current Knowledge changes cannot alter this evidence closure.
func (r *EvidenceReader) ReportEvidence(
	ctx context.Context,
	selected querybase.Epoch,
) (_ queryglobal.ReportEvidence, resultErr error) {
	if r == nil || r.reports == nil || r.knowledge == nil || r.metadata == nil {
		return queryglobal.ReportEvidence{}, errors.New("Global evidence reader is not configured")
	}
	reportView, err := r.reports.Publication(ctx, selected)
	if err != nil {
		return queryglobal.ReportEvidence{}, err
	}
	result := queryglobal.ReportEvidence{ReportView: reportView}
	references := entityReferences(reportView.Reports)
	if len(references) == 0 {
		return result, nil
	}
	view, err := r.knowledge.OpenCurrent(ctx)
	if err != nil {
		return queryglobal.ReportEvidence{}, fmt.Errorf("open Global Knowledge view: %w", err)
	}
	if view == nil {
		return queryglobal.ReportEvidence{}, errors.New("Knowledge returned a nil Read View")
	}
	defer func() { resultErr = errors.Join(resultErr, view.Close()) }()
	entityReader := view.Entities()
	if entityReader == nil {
		return queryglobal.ReportEvidence{}, errors.New("Knowledge returned a nil Entity reader")
	}
	versions, err := entityReader.Read(ctx, references)
	if err != nil {
		return queryglobal.ReportEvidence{}, fmt.Errorf("read Global Entity evidence: %w", err)
	}
	if len(versions) != len(references) {
		return queryglobal.ReportEvidence{}, errors.New("Global Entity Version set is incomplete")
	}
	entities := make([]queryglobal.EntityEvidence, len(versions))
	for index, version := range versions {
		reference := references[index]
		if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
			return queryglobal.ReportEvidence{}, errors.New("Global Entity differs from its exact Version reference")
		}
		metadata, err := r.metadata.Entity(ctx, reference)
		if err != nil {
			return queryglobal.ReportEvidence{}, err
		}
		entities[index] = queryglobal.EntityEvidence{
			Reference: querybase.KnowledgeReference{
				ID: string(version.Knowledge.ID), Version: uint64(version.Version),
			},
			TextUnitIDs: evidenceTextUnitIDs(metadata.Evidence),
		}
	}
	result.Entities = entities
	return result, nil
}

func evidenceTextUnitIDs(evidence []provenance.Evidence) []string {
	unique := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		unique[item.TextUnitID] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for textUnitID := range unique {
		result = append(result, textUnitID)
	}
	sort.Strings(result)
	return result
}

func entityReferences(reports []queryreport.PublishedReport) []knowledge.Reference[knowledge.EntityID] {
	unique := make(map[knowledge.Reference[knowledge.EntityID]]struct{})
	for _, report := range reports {
		for _, source := range report.Sources.Entities {
			unique[knowledge.Reference[knowledge.EntityID]{
				ID: knowledge.EntityID(source.ID), Version: knowledge.Version(source.Version),
			}] = struct{}{}
		}
	}
	result := make([]knowledge.Reference[knowledge.EntityID], 0, len(unique))
	for reference := range unique {
		result = append(result, reference)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].ID != result[right].ID {
			return result[left].ID < result[right].ID
		}
		return result[left].Version < result[right].Version
	})
	return result
}
