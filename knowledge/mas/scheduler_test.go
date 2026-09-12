package mas_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/memoria-space/meking/knowledge/mas"
)

func TestReferenceTransitions(t *testing.T) {
	type estimate struct {
		Stability  float64    `json:"stability"`
		Difficulty float64    `json:"difficulty"`
		LastReview *time.Time `json:"last_review"`
	}
	type probe struct {
		At         time.Time `json:"at"`
		Activation float64   `json:"activation"`
	}
	var fixture struct {
		Version string `json:"version"`
		Cases   []struct {
			Name       string      `json:"name"`
			Parameters [21]float64 `json:"parameters"`
			Before     estimate    `json:"before"`
			Grade      mas.Grade   `json:"grade"`
			At         time.Time   `json:"at"`
			Activation float64     `json:"activation"`
			After      estimate    `json:"after"`
			Probes     []probe     `json:"probes"`
		} `json:"cases"`
	}
	contents, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "6.3.2" || len(fixture.Cases) != 824 {
		t.Fatalf("unexpected reference version/count: %s/%d", fixture.Version, len(fixture.Cases))
	}
	if mas.DefaultConfig().Parameters != fixture.Cases[0].Parameters {
		t.Fatal("default parameters differ from the pinned reference")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			scheduler, err := mas.NewScheduler(mas.Config{Parameters: tc.Parameters})
			if err != nil {
				t.Fatal(err)
			}
			before := mas.State{Stability: mas.Stability(tc.Before.Stability), Difficulty: mas.Difficulty(tc.Before.Difficulty)}
			if tc.Before.LastReview != nil {
				before.LastReview = *tc.Before.LastReview
			}
			activation, err := scheduler.CalcLiveness(before, tc.At)
			if err != nil {
				t.Fatal(err)
			}
			closeNumber(t, "activation before", activation.Float64(), tc.Activation)
			after, err := scheduler.UpdateLiveness(before, tc.Grade, tc.At)
			if err != nil {
				t.Fatal(err)
			}
			closeNumber(t, "stability", after.Stability.Float64(), tc.After.Stability)
			closeNumber(t, "difficulty", after.Difficulty.Float64(), tc.After.Difficulty)
			if tc.After.LastReview == nil || !after.LastReview.Equal(*tc.After.LastReview) {
				t.Fatalf("last review: %v != %v", after.LastReview, tc.After.LastReview)
			}
			if tc.Before.LastReview == nil {
				seeded, err := scheduler.SeedState(tc.Grade, tc.At)
				if err != nil || seeded != after {
					t.Fatalf("initialization mismatch: %+v, %v", seeded, err)
				}
			}
			for _, p := range tc.Probes {
				actual, err := scheduler.CalcLiveness(after, p.At)
				if err != nil {
					t.Fatal(err)
				}
				closeNumber(t, "activation probe", actual.Float64(), p.Activation)
			}
		})
	}
}

func closeNumber(t *testing.T, name string, actual, expected float64) {
	t.Helper()
	if math.IsNaN(actual) || math.IsInf(actual, 0) || math.Abs(actual-expected) > 1e-12*math.Max(1, math.Abs(expected)) {
		t.Fatalf("%s: got %.17g, want %.17g", name, actual, expected)
	}
}

func TestDefaultParametersAndIsolation(t *testing.T) {
	cfg := mas.DefaultConfig()
	scheduler, err := mas.NewScheduler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	before, err := scheduler.SeedState(mas.GradeGood, at)
	if err != nil {
		t.Fatal(err)
	}
	closeNumber(t, "initial good stability", before.Stability.Float64(), 2.3065)
	closeNumber(t, "initial good difficulty", before.Difficulty.Float64(), 2.118103970459015)
	cfg.Parameters[2] = 100
	after, err := scheduler.SeedState(mas.GradeGood, at)
	if err != nil || before != after {
		t.Fatalf("configuration mutation reached scheduler: %+v, %v", after, err)
	}
}

func TestInvalidParameters(t *testing.T) {
	for index := range mas.DefaultConfig().Parameters {
		for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1, 1e6} {
			cfg := mas.DefaultConfig()
			cfg.Parameters[index] = invalid
			if cfg.Validate() == nil {
				t.Fatalf("parameter %d accepted %v", index, invalid)
			}
			if _, err := mas.NewScheduler(cfg); err == nil {
				t.Fatalf("constructor accepted parameter %d = %v", index, invalid)
			}
		}
	}
}

func TestStateAndTimeValidation(t *testing.T) {
	scheduler, err := mas.NewScheduler(mas.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	valid, err := scheduler.SeedState(mas.GradeGood, at)
	if err != nil {
		t.Fatal(err)
	}
	invalidStates := []mas.State{
		{Stability: 3}, {Difficulty: 5}, {LastReview: at},
		{Stability: -1, Difficulty: 5, LastReview: at},
		{Stability: mas.Stability(math.NaN()), Difficulty: 5, LastReview: at},
		{Stability: 3, Difficulty: mas.Difficulty(math.Inf(1)), LastReview: at},
		{Stability: 3, Difficulty: 11, LastReview: at},
		{Stability: 0.0001, Difficulty: 5, LastReview: at},
		{Stability: 3, Difficulty: 5},
	}
	for _, state := range invalidStates {
		if state.Validate() == nil {
			t.Fatalf("accepted state %+v", state)
		}
		if _, err := scheduler.CalcLiveness(state, at); err == nil {
			t.Fatalf("activation accepted %+v", state)
		}
		if _, err := scheduler.UpdateLiveness(state, mas.GradeGood, at); err == nil {
			t.Fatalf("review accepted %+v", state)
		}
	}
	for _, grade := range []mas.Grade{0, 5, -1} {
		if _, err := scheduler.SeedState(grade, at); err == nil {
			t.Fatalf("accepted seed grade %d", grade)
		}
		if _, err := scheduler.UpdateLiveness(valid, grade, at); err == nil {
			t.Fatalf("accepted grade %d", grade)
		}
	}
	if _, err := scheduler.UpdateLiveness(valid, mas.GradeGood, at.Add(-time.Nanosecond)); err == nil {
		t.Fatal("accepted out-of-order review")
	}
	if _, err := scheduler.UpdateLiveness(valid, mas.GradeGood, time.Time{}); err == nil {
		t.Fatal("accepted missing review time")
	}
	if _, err := scheduler.SeedState(mas.GradeGood, time.Time{}); err == nil {
		t.Fatal("accepted missing seed time")
	}
	if _, err := scheduler.CalcLiveness(valid, time.Time{}); err == nil {
		t.Fatal("accepted missing evaluation time")
	}
	if _, err := scheduler.CalcLiveness(mas.State{}, at); err != nil {
		t.Fatal(err)
	}
	if _, err := (&mas.Scheduler{}).SeedState(mas.GradeGood, at); err == nil {
		t.Fatal("accepted unconfigured scheduler")
	}
}

func TestElapsedWholeDaysAndLongHistory(t *testing.T) {
	scheduler, err := mas.NewScheduler(mas.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	last := time.Date(2026, 1, 1, 23, 59, 59, 123456789, time.FixedZone("offset", 8*3600))
	state := mas.State{Stability: 1, Difficulty: 5, LastReview: last}
	for _, tc := range []struct {
		at   time.Time
		want float64
	}{
		{last.Add(-time.Hour), 1}, {last.Add(time.Hour), 1},
		{last.Add(24*time.Hour - time.Nanosecond), 1}, {last.Add(24 * time.Hour), 0.9},
	} {
		value, err := scheduler.CalcLiveness(state, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		closeNumber(t, "whole day", value.Float64(), tc.want)
	}
	state.LastReview = time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC)
	near, err := scheduler.CalcLiveness(state, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	far, err := scheduler.CalcLiveness(state, time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if far >= near {
		t.Fatalf("elapsed time saturated: %g >= %g", far, near)
	}
}
