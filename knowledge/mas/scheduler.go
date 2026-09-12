// Package mas computes memory stability, difficulty, and activation from
// observed recall results. Identity, evidence, and persistence belong to callers.
package mas

import (
	"errors"
	"math"
	"time"
)

type Scheduler struct {
	cfg Config
}

func NewScheduler(cfg Config) (*Scheduler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Scheduler{cfg: cfg}, nil
}

// State is a replayable memory estimate. The all-zero value means no recall
// has been observed; a partially initialized state is invalid.
type State struct {
	Stability  Stability
	Difficulty Difficulty
	LastReview time.Time
}

func (state State) Validate() error {
	if state == (State{}) {
		return nil
	}
	if _, err := NewStability(state.Stability.Float64()); err != nil {
		return err
	}
	if _, err := NewDifficulty(state.Difficulty.Float64()); err != nil {
		return err
	}
	return validateTime(state.LastReview)
}

// CalcLiveness uses complete elapsed 24-hour periods. Missing memory history
// yields zero activation; a time before the last review yields full activation.
func (s *Scheduler) CalcLiveness(state State, now time.Time) (Activation, error) {
	if err := s.cfg.Validate(); err != nil {
		return 0, err
	}
	if err := state.Validate(); err != nil {
		return 0, err
	}
	if err := validateTime(now); err != nil {
		return 0, err
	}
	if state == (State{}) {
		return 0, nil
	}
	return NewActivation(s.activation(state.Stability.Float64(), elapsedDays(state.LastReview, now)))
}

// UpdateLiveness applies one observed recall. Callers deduplicate observations
// before invoking it; distinct observations at the same time are valid.
func (s *Scheduler) UpdateLiveness(state State, grade Grade, at time.Time) (State, error) {
	if err := s.cfg.Validate(); err != nil {
		return State{}, err
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	if _, err := NewGrade(grade.Int()); err != nil {
		return State{}, err
	}
	if err := validateTime(at); err != nil {
		return State{}, err
	}
	if state == (State{}) {
		return s.SeedState(grade, at)
	}
	if at.Before(state.LastReview) {
		return State{}, errors.New("mas: review predates last review")
	}

	stability := state.Stability.Float64()
	difficulty := state.Difficulty.Float64()
	days := elapsedDays(state.LastReview, at)
	var next float64
	switch {
	case days < 1:
		next = s.shortTermStability(stability, grade)
	case grade == GradeAgain:
		next = s.forgetStability(stability, difficulty, s.activation(stability, days))
	default:
		next = s.recallStability(stability, difficulty, s.activation(stability, days), grade)
	}
	return newState(next, s.nextDifficulty(difficulty, grade), at)
}

// SeedState derives the first memory estimate from the first observed grade.
func (s *Scheduler) SeedState(grade Grade, at time.Time) (State, error) {
	if err := s.cfg.Validate(); err != nil {
		return State{}, err
	}
	if _, err := NewGrade(grade.Int()); err != nil {
		return State{}, err
	}
	if err := validateTime(at); err != nil {
		return State{}, err
	}
	return newState(s.cfg.Parameters[grade-1], s.initialDifficulty(grade), at)
}

func newState(stability, difficulty float64, at time.Time) (State, error) {
	// Reject failed calculations before applying the model's finite bounds.
	if math.IsNaN(stability) || math.IsInf(stability, 0) || math.IsNaN(difficulty) || math.IsInf(difficulty, 0) {
		return State{}, errors.New("mas: memory calculation must be finite")
	}
	value := State{
		Stability:  Stability(math.Max(minStability, stability)),
		Difficulty: Difficulty(math.Min(10, math.Max(1, difficulty))),
		LastReview: at.UTC(),
	}
	if err := value.Validate(); err != nil {
		return State{}, err
	}
	return value, nil
}

func (s *Scheduler) activation(stability, days float64) float64 {
	decay := -s.cfg.Parameters[20]
	factor := math.Pow(0.9, 1/decay) - 1
	return math.Pow(1+factor*days/stability, decay)
}

func (s *Scheduler) initialDifficulty(grade Grade) float64 {
	w := s.cfg.Parameters
	return w[4] - math.Pow(math.E, w[5]*float64(grade-1)) + 1
}

func (s *Scheduler) nextDifficulty(difficulty float64, grade Grade) float64 {
	w := s.cfg.Parameters
	delta := -w[6] * float64(grade-3)
	adjusted := difficulty + (10-difficulty)*delta/9
	// The reversion target is the unbounded initial easy difficulty. Clamp
	// only the final result, otherwise the long-run trajectory changes.
	return w[7]*s.initialDifficulty(GradeEasy) + (1-w[7])*adjusted
}

func (s *Scheduler) shortTermStability(stability float64, grade Grade) float64 {
	w := s.cfg.Parameters
	increase := math.Pow(math.E, w[17]*(float64(grade-3)+w[18])) * math.Pow(stability, -w[19])
	if grade != GradeAgain {
		increase = math.Max(increase, 1)
	}
	return stability * increase
}

func (s *Scheduler) recallStability(stability, difficulty, activation float64, grade Grade) float64 {
	w := s.cfg.Parameters
	boost := 1.0
	if grade == GradeHard {
		boost = w[15]
	} else if grade == GradeEasy {
		boost = w[16]
	}
	return stability * (1 + math.Pow(math.E, w[8])*(11-difficulty)*math.Pow(stability, -w[9])*
		(math.Pow(math.E, (1-activation)*w[10])-1)*boost)
}

func (s *Scheduler) forgetStability(stability, difficulty, activation float64) float64 {
	w := s.cfg.Parameters
	longTerm := w[11] * math.Pow(difficulty, -w[12]) * (math.Pow(stability+1, w[13]) - 1) *
		math.Pow(math.E, (1-activation)*w[14])
	shortTerm := stability / math.Pow(math.E, w[17]*w[18])
	return math.Min(longTerm, shortTerm)
}

func validateTime(at time.Time) error {
	if at.IsZero() || at.UTC().Year() < 1 || at.UTC().Year() > 9999 {
		return errors.New("mas: time must be set and within years 1..9999")
	}
	return nil
}

func elapsedDays(last, now time.Time) float64 {
	if !now.After(last) {
		return 0
	}
	// Sub saturates beyond roughly 292 years. Subtract integer seconds first,
	// preserving the day boundary even for the full supported date range.
	seconds := now.Unix() - last.Unix()
	if now.Nanosecond() < last.Nanosecond() {
		seconds--
	}
	return float64(seconds / 86400)
}
