package vectorindex

import (
	"context"
	"errors"
	"slices"

	communityreport "github.com/memoria-space/meking/community/report"
	semanticbase "github.com/memoria-space/meking/semantic"
)

type Service struct {
	semantic *semanticbase.Service
}

type Validator struct {
	store semanticbase.NamespaceStore
}

func New(service *semanticbase.Service) (*Service, error) {
	if service == nil {
		return nil, errors.New("create Report vector Service: Semantic Service is required")
	}
	return &Service{semantic: service}, nil
}

func NewValidator(store semanticbase.NamespaceStore) (*Validator, error) {
	if store == nil {
		return nil, errors.New("create Report vector Validator: NamespaceStore is required")
	}
	return &Validator{store: store}, nil
}

func (s *Service) Build(
	ctx context.Context,
	set communityreport.ReportSet,
	reports []communityreport.Report,
) (int, error) {
	sources, namespace, err := prepare(set, reports)
	if err != nil {
		return 0, err
	}
	if len(sources) == 0 {
		return 0, s.semantic.Add(ctx, namespace, nil)
	}
	for start := 0; start < len(sources); start += PageSize {
		end := min(start+PageSize, len(sources))
		input := make([]semanticbase.Input, end-start)
		for index, source := range sources[start:end] {
			input[index] = semanticbase.Input{ID: source.ID, Text: source.Text}
		}
		if err = s.semantic.Add(ctx, namespace, input); err != nil {
			return 0, err
		}
	}
	return s.validate(ctx, namespace, sources)
}

func (s *Service) Validate(ctx context.Context, set communityreport.ReportSet) error {
	sources, namespace, err := membership(set)
	if err != nil {
		return err
	}
	_, err = s.validate(ctx, namespace, sources)
	return err
}

func (s *Service) validate(
	ctx context.Context,
	namespace semanticbase.Namespace,
	sources []Source,
) (int, error) {
	reader, err := s.semantic.Open(ctx, namespace)
	if err != nil {
		return 0, mapNotFound(err)
	}
	return len(sources), reader.Close()
}

func (s *Service) Delete(ctx context.Context, reportSetID string) error {
	name, err := Namespace(communityreport.ReportSetID(reportSetID))
	if err != nil {
		return err
	}
	return s.semantic.Delete(ctx, semanticbase.Namespace(name))
}

func (v *Validator) Validate(ctx context.Context, set communityreport.ReportSet) error {
	_, namespace, err := membership(set)
	if err != nil {
		return err
	}
	reader, err := v.store.Open(ctx, namespace)
	if err != nil {
		return mapNotFound(err)
	}
	return reader.Close()
}

func prepare(
	set communityreport.ReportSet,
	reports []communityreport.Report,
) ([]Source, semanticbase.Namespace, error) {
	name, err := Namespace(set.ID)
	if err != nil {
		return nil, "", err
	}
	sources, err := Sources(set, reports)
	return sources, semanticbase.Namespace(name), err
}

func membership(
	set communityreport.ReportSet,
) ([]Source, semanticbase.Namespace, error) {
	if err := communityreport.ValidateReportSet(set); err != nil {
		return nil, "", err
	}
	name, err := Namespace(set.ID)
	if err != nil {
		return nil, "", err
	}
	ids := make([]communityreport.ID, 0, len(set.Reports))
	for _, id := range set.Reports {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	sources := make([]Source, len(ids))
	for index, id := range ids {
		sources[index] = Source{ID: string(id)}
	}
	return sources, semanticbase.Namespace(name), nil
}

func mapNotFound(err error) error {
	if errors.Is(err, semanticbase.ErrNamespaceNotFound) {
		return errors.Join(ErrNotFound, err)
	}
	return err
}
