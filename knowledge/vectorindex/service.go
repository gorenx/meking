package vectorindex

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/memoria-space/meking/knowledge"
	semanticbase "github.com/memoria-space/meking/semantic"
)

type Service struct {
	sources  *Sources
	semantic *semanticbase.Service
}

type Validator struct {
	sources *Sources
	store   semanticbase.NamespaceStore
}

func New(source Knowledge, service *semanticbase.Service) (*Service, error) {
	if service == nil {
		return nil, errors.New("create Entity vector Service: Semantic Service is required")
	}
	sources, err := NewSources(source)
	if err != nil {
		return nil, err
	}
	return &Service{
		sources:  sources,
		semantic: service,
	}, nil
}

func NewValidator(source Knowledge, store semanticbase.NamespaceStore) (*Validator, error) {
	if store == nil {
		return nil, errors.New("create Entity vector Validator: Namespace Store is required")
	}
	sources, err := NewSources(source)
	if err != nil {
		return nil, err
	}
	return &Validator{
		sources: sources,
		store:   store,
	}, nil
}

func (service *Service) Build(ctx context.Context, target Target) (int, error) {
	namespace, err := Namespace(target.Entities)
	if err != nil {
		return 0, err
	}
	for offset := 0; ; offset += PageSize {
		page, err := service.sources.Page(ctx, target.Entities, offset)
		if err != nil {
			return 0, err
		}
		input := make([]semanticbase.Input, len(page.Items))
		for index, source := range page.Items {
			input[index] = semanticbase.Input{
				ID:   source.ID,
				Text: source.Text,
			}
		}
		if err := service.semantic.Add(ctx, semanticbase.Namespace(namespace), input); err != nil {
			return 0, fmt.Errorf("add Entity vectors: %w", err)
		}
		if !page.HasMore {
			break
		}
	}
	return service.validate(ctx, target.Entities)
}

func (service *Service) Validate(
	ctx context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) error {
	_, err := service.validate(ctx, entities)
	return err
}

func (service *Service) validate(
	ctx context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) (int, error) {
	namespace, err := Namespace(entities)
	if err != nil {
		return 0, err
	}
	reader, err := service.semantic.Open(ctx, semanticbase.Namespace(namespace))
	if err != nil {
		return 0, mapNotFound(err)
	}
	return validateReader(ctx, service.sources, reader, entities, namespace)
}

func (service *Service) Delete(
	ctx context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) error {
	namespace, err := Namespace(entities)
	if err != nil {
		return err
	}
	return service.semantic.Delete(ctx, semanticbase.Namespace(namespace))
}

func (validator *Validator) Validate(
	ctx context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
) error {
	namespace, err := Namespace(entities)
	if err != nil {
		return err
	}
	reader, err := validator.store.Open(ctx, semanticbase.Namespace(namespace))
	if err != nil {
		return mapNotFound(err)
	}
	_, err = validateReader(ctx, validator.sources, reader, entities, namespace)
	return err
}

func validateReader(
	ctx context.Context,
	sources *Sources,
	reader semanticbase.NamespaceReader,
	entities []knowledge.Reference[knowledge.EntityID],
	namespace string,
) (_ int, resultErr error) {
	defer func() {
		resultErr = errors.Join(resultErr, reader.Close())
	}()
	info, err := reader.Info(ctx)
	if err != nil {
		return 0, err
	}
	expectedNamespace := semanticbase.Namespace(namespace)
	if info.Namespace != expectedNamespace {
		return 0, fmt.Errorf(
			"Entity vector reader opened Namespace %q; expected %q",
			info.Namespace,
			expectedNamespace,
		)
	}
	count := 0
	vectorAfter := ""
	for {
		expected, err := sources.Page(ctx, entities, count)
		if err != nil {
			return 0, err
		}
		actual, err := reader.IDs(ctx, vectorAfter, PageSize)
		if err != nil {
			return 0, err
		}
		expectedIDs := make([]string, len(expected.Items))
		for index, source := range expected.Items {
			expectedIDs[index] = source.ID
		}
		if !slices.Equal(expectedIDs, actual.Items) || expected.HasMore != actual.HasMore {
			return 0, fmt.Errorf(
				"Entity vector Namespace %q has different membership at offset %d",
				namespace,
				count,
			)
		}
		count += len(expectedIDs)
		if !expected.HasMore {
			break
		}
		vectorAfter = actual.NextAfter
	}
	return count, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, semanticbase.ErrNamespaceNotFound) {
		return errors.Join(ErrNotFound, err)
	}
	return err
}
