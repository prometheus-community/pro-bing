package probing

import (
	"math"
	"time"
)

// TODO: annotate properly
// TODO: use in ping
// welford's online method for stddev
// https://en.wikipedia.org/wiki/Algorithms_for_calculating_variance#Welford's_online_algorithm
func calculateStdDev(totalCount int, newVal, avg, stdDevM2 time.Duration) (time.Duration, time.Duration, time.Duration) {
	delta := newVal - avg
	avg += delta / time.Duration(totalCount)
	delta2 := newVal - avg
	stdDevM2 += delta * delta2
	stdDev := time.Duration(math.Sqrt(float64(stdDevM2 / time.Duration(totalCount))))
	return stdDev, stdDevM2, avg
}
