package semantic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	DefaultBatchSize      = 16
	DefaultBatchMaxTokens = 8191
)

var (
	ErrNamespaceNotFound = errors.New("Semantic Namespace not found")
)

// Namespace identifies one caller-owned vector membership. Its value is opaque
// to Semantic and is assigned by the source domain.
type Namespace string

// Input binds an opaque vector identity to the complete text embedded for it.
type Input struct {
	ID   string
	Text string
}

type EmbeddingBatch struct {
	Vectors [][]float64
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) (EmbeddingBatch, error)
}

type tokenCounter interface {
	Count(text string) (int, error)
}

type Vector struct {
	ID     string
	Values []float64
}

type Info struct {
	Namespace Namespace
	Model     string
	Dimension int
}

type IDPage struct {
	Items     []string
	NextAfter string
	HasMore   bool
}

type Match struct {
	ID    string
	Score float64
}

type GenerationConfig struct {
	BatchSize      int
	BatchMaxTokens int
	MaxConcurrency int
}

func DefaultGenerationConfig() GenerationConfig {
	return GenerationConfig{
		BatchSize: DefaultBatchSize, BatchMaxTokens: DefaultBatchMaxTokens,
		MaxConcurrency: 1,
	}
}

// NamespaceStore persists caller-owned vector membership and fixed reads.
type NamespaceStore interface {
	Add(ctx context.Context, namespace Namespace, vectors []Vector) error
	Open(ctx context.Context, namespace Namespace) (NamespaceReader, error)
	Delete(ctx context.Context, namespace Namespace) error
}

type NamespaceReader interface {
	Info(ctx context.Context) (Info, error)
	IDs(ctx context.Context, after string, limit int) (IDPage, error)
	Search(ctx context.Context, vector []float64, limit int, filter map[string]string) ([]Match, error)
	Close() error
}

func ValidateNamespace(namespace Namespace) error {
	return ValidateID(string(namespace), "Namespace")
}

func ValidateID(value string, name string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("Semantic %s is required", name)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("Semantic %s must not have surrounding whitespace", name)
	}
	return nil
}

func PrepareIDs(ids []string) ([]string, error) {
	prepared := append([]string(nil), ids...)
	seen := make(map[string]struct{}, len(prepared))
	for index, id := range prepared {
		if err := ValidateID(id, "vector ID"); err != nil {
			return nil, fmt.Errorf("vector ID %d: %w", index, err)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("Semantic vector ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return prepared, nil
}

func ValidateVector(values []float64) error {
	if len(values) == 0 {
		return errors.New("Semantic vector is empty")
	}
	norm := 0.0
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("Semantic vector contains a non-finite value")
		}
		norm += value * value
	}
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return errors.New("Semantic vector has no finite magnitude")
	}
	return nil
}

func NormalizeVector(values []float64) ([]float64, error) {
	if err := ValidateVector(values); err != nil {
		return nil, err
	}
	norm := 0.0
	for _, value := range values {
		norm += value * value
	}
	norm = math.Sqrt(norm)
	result := make([]float64, len(values))
	for index, value := range values {
		result[index] = value / norm
	}
	return result, nil
}
