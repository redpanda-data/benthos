package llmtest

import (
	"math"
	"sort"
)

// Stats contains statistical aggregates for a multi-judge evaluation.
type Stats struct {
	Mean   float64
	StdDev float64
	Median float64
	Min    int
	Max    int
}

func computeStats(scores []int) Stats {
	if len(scores) == 0 {
		return Stats{}
	}

	sorted := make([]int, len(scores))
	copy(sorted, scores)
	sort.Ints(sorted)

	sum := 0
	for _, s := range sorted {
		sum += s
	}
	mean := float64(sum) / float64(len(sorted))

	varSum := 0.0
	for _, s := range sorted {
		d := float64(s) - mean
		varSum += d * d
	}
	stddev := math.Sqrt(varSum / float64(len(sorted)))

	var median float64
	n := len(sorted)
	if n%2 == 0 {
		median = float64(sorted[n/2-1]+sorted[n/2]) / 2.0
	} else {
		median = float64(sorted[n/2])
	}

	return Stats{
		Mean:   mean,
		StdDev: stddev,
		Median: median,
		Min:    sorted[0],
		Max:    sorted[n-1],
	}
}
