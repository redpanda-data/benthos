package llmtest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeStats_Empty(t *testing.T) {
	s := computeStats(nil)
	assert.Equal(t, Stats{}, s)
}

func TestComputeStats_Single(t *testing.T) {
	s := computeStats([]int{75})
	assert.Equal(t, 75.0, s.Mean)
	assert.Equal(t, 0.0, s.StdDev)
	assert.Equal(t, 75.0, s.Median)
	assert.Equal(t, 75, s.Min)
	assert.Equal(t, 75, s.Max)
}

func TestComputeStats_Odd(t *testing.T) {
	s := computeStats([]int{60, 80, 90})
	assert.InDelta(t, 76.6667, s.Mean, 0.001)
	assert.InDelta(t, 12.472, s.StdDev, 0.01)
	assert.Equal(t, 80.0, s.Median)
	assert.Equal(t, 60, s.Min)
	assert.Equal(t, 90, s.Max)
}

func TestComputeStats_Even(t *testing.T) {
	s := computeStats([]int{50, 60, 70, 80})
	assert.Equal(t, 65.0, s.Mean)
	assert.Equal(t, 65.0, s.Median)
	assert.Equal(t, 50, s.Min)
	assert.Equal(t, 80, s.Max)

	// Population stddev of [50,60,70,80]
	expectedStdDev := math.Sqrt(((15*15 + 5*5 + 5*5 + 15*15) / 4.0))
	assert.InDelta(t, expectedStdDev, s.StdDev, 0.001)
}

func TestComputeStats_AllSame(t *testing.T) {
	s := computeStats([]int{42, 42, 42})
	assert.Equal(t, 42.0, s.Mean)
	assert.Equal(t, 0.0, s.StdDev)
	assert.Equal(t, 42.0, s.Median)
	assert.Equal(t, 42, s.Min)
	assert.Equal(t, 42, s.Max)
}

func TestComputeStats_DoesNotMutateInput(t *testing.T) {
	scores := []int{90, 10, 50}
	computeStats(scores)
	assert.Equal(t, []int{90, 10, 50}, scores)
}
