package mas

import (
	"fmt"
	"math"
)

type Config struct {
	// Parameters uses zero-based indexes with the following fixed meanings:
	//   0..3: initial stability in days for Again, Hard, Good, and Easy.
	//   4: initial difficulty for Again, before bounding it to [1, 10].
	//   5: grade sensitivity of the initial difficulty curve.
	//   6: grade-driven difficulty adjustment strength.
	//   7: difficulty mean-reversion weight toward the unbounded easy anchor.
	//   8: log scale of the long-term successful-recall stability gain.
	//   9: stability exponent reducing relative long-term gain as S grows.
	//   10: sensitivity of successful-recall gain to lost activation (1-R).
	//   11: scale of the long-term post-failure stability estimate.
	//   12: difficulty exponent in the post-failure estimate.
	//   13: prior-stability exponent in the post-failure estimate.
	//   14: sensitivity of post-failure stability to lost activation (1-R).
	//   15: Hard multiplier for long-term successful-recall gain.
	//   16: Easy multiplier for long-term successful-recall gain.
	//   17: grade sensitivity of short-term stability gain.
	//   18: grade offset in short-term gain; with 17 also bounds failure recovery.
	//   19: stability exponent damping short-term growth as S grows.
	//   20: positive decay magnitude; also derives the factor making R(S,S)=0.9.
	// Except for the initial day values, these are coefficients calibrated with
	// stability measured in days. Validate enforces each index's bounds. The
	// array is copied into Scheduler, isolating it from later configuration edits.
	Parameters [21]float64
}

func DefaultConfig() Config {
	return Config{Parameters: [21]float64{
		0.212, 1.2931, 2.3065, 8.2956, 6.4133, 0.8334, 3.0194,
		0.001, 1.8722, 0.1666, 0.796, 1.4835, 0.0614, 0.2629,
		1.6483, 0.6014, 1.8729, 0.5425, 0.0912, 0.0658, 0.1542,
	}}
}

func (c Config) Validate() error {
	lower := [21]float64{
		minStability, minStability, minStability, minStability,
		1, 0.001, 0.001, 0.001, 0, 0, 0.001, 0.001, 0.001,
		0.001, 0, 0, 1, 0, 0, 0, 0.1,
	}
	upper := [21]float64{
		100, 100, 100, 100, 10, 4, 4, 0.75, 4.5, 0.8, 3.5,
		5, 0.25, 0.9, 4, 1, 6, 2, 2, 0.8, 0.8,
	}
	for index, weight := range c.Parameters {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < lower[index] || weight > upper[index] {
			return fmt.Errorf("mas: parameter %d must be finite and within [%g, %g]", index, lower[index], upper[index])
		}
	}
	return nil
}
