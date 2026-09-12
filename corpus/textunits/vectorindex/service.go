package vectorindex

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/corpus/textunits"
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
		return nil, errors.New("create TextUnit vector Service: Semantic Service is required")
	}
	return &Service{semantic: service}, nil
}

func NewValidator(store semanticbase.NamespaceStore) (*Validator, error) {
	if store == nil {
		return nil, errors.New("create TextUnit vector Validator: NamespaceStore is required")
	}
	return &Validator{store: store}, nil
}

func (s *Service) Build(
	ctx context.Context,
	corporaID string,
	units []textunits.TextUnitBody,
) (int, error) {
	sources, namespace, err := prepare(corporaID, units)
	if err != nil {
		return 0, err
	}
	if err = s.add(ctx, namespace, sources); err != nil {
		return 0, fmt.Errorf("add TextUnit vectors: %w", err)
	}
	return s.validate(ctx, namespace, sources)
}

func (s *Service) add(
	ctx context.Context,
	namespace semanticbase.Namespace,
	sources []Source,
) error {

	if len(sources) == 0 {
		return s.semantic.Add(ctx, namespace, nil)
	}
	for start := 0; start < len(sources); start += PageSize {
		end := min(start+PageSize, len(sources))
		input := make([]semanticbase.Input, end-start)
		for index, source := range sources[start:end] {
			input[index] = semanticbase.Input{ID: source.ID, Text: source.Text}
		}
		if err := s.semantic.Add(ctx, namespace, input); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Validate(
	ctx context.Context,
	corporaID string,
	units []textunits.TextUnitBody,
) error {
	sources, namespace, err := prepare(corporaID, units)
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

func (s *Service) Delete(ctx context.Context, corporaID string) error {
	name, err := Namespace(corporaID)
	if err != nil {
		return err
	}
	return s.semantic.Delete(ctx, semanticbase.Namespace(name))
}

func (v *Validator) Validate(
	ctx context.Context,
	corporaID string,
	units []textunits.TextUnitBody,
) error {
	_, namespace, err := prepare(corporaID, units)
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
	corporaID string,
	units []textunits.TextUnitBody,
) ([]Source, semanticbase.Namespace, error) {
	name, err := Namespace(corporaID)
	if err != nil {
		return nil, "", err
	}
	sources, err := Sources(units)
	return sources, semanticbase.Namespace(name), err
}

func mapNotFound(err error) error {
	if errors.Is(err, semanticbase.ErrNamespaceNotFound) {
		return errors.Join(ErrNotFound, err)
	}
	return err
}
