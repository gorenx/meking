package mas

import (
	"errors"
	"math"
)

const minStability = 0.001

// Stability is the decay time scale in days, not the importance of knowledge.
type Stability float64

func NewStability(v float64) (Stability, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < minStability {
		return 0, errors.New("mas: stability must be finite and at least 0.001 days")
	}
	return Stability(v), nil
}

func (s Stability) Float64() float64 { return float64(s) }

// Difficulty controls how hard it is to increase stability. It does not
// measure how often the underlying knowledge changes.
type Difficulty float64

func NewDifficulty(v float64) (Difficulty, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 1 || v > 10 {
		return 0, errors.New("mas: difficulty must be finite and within [1, 10]")
	}
	return Difficulty(v), nil
}

func (d Difficulty) Float64() float64 { return float64(d) }

// Activation is a model estimate of recall availability, not relevance or truth.
type Activation float64

func NewActivation(v float64) (Activation, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		return 0, errors.New("mas: activation must be finite and within [0, 1]")
	}
	return Activation(v), nil
}

func (a Activation) Float64() float64 { return float64(a) }
