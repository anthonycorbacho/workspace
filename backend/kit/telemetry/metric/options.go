package metric

import (
	"sort"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
)

// An Option is used to construct a metric.
type Option func(c *metric) error

// Labels describes the label names of this metric.
func Labels(labels ...string) Option {
	return func(m *metric) error {
		m.labels = append(m.labels, labels...)
		return nil
	}
}

// Histogram counts individual observations from an event or sample stream in configurable buckets.
//
// Warning: Histograms are expensive and should be used sparingly.
func Histogram(buckets ...float64) Option {
	return func(m *metric) error {
		if len(buckets) < 1 {
			return errors.New("empty buckets")
		}
		sort.Float64s(buckets)
		for i := 0; i < len(buckets)-1; i++ {
			if buckets[i+1] <= buckets[i] {
				return errors.New("buckets must be defined in ascending order")
			}
		}
		m.kind = histogram
		m.buckets = buckets
		return nil
	}
}

// Summary captures individual observations from an event or sample stream and
// summarizes them in a manner similar to traditional summary statistics:
// 1. sum of observations
// 2. observation count
// 3. rank estimations.
//
// Warning: Summaries with objectives are expensive and should be used sparingly.
func Summary(objectives map[float64]float64) Option {
	return func(m *metric) error {
		m.kind = summary
		m.objectives = objectives
		return nil
	}
}

// MaxAge defines the duration for which an observation stays relevant for summary metrics.
// Must be positive.
func MaxAge(maxAge time.Duration) Option {
	return func(m *metric) error {
		if maxAge <= 0 {
			return errors.New("max age must be positive")
		}
		m.maxAge = maxAge
		return nil
	}
}

// Gauge represents a single numerical value that can arbitrarily go up and down.
func Gauge() Option {
	return func(m *metric) error {
		m.kind = gauge
		return nil
	}
}

// Counter represents a single numerical value that only ever goes up.
func Counter() Option {
	return func(m *metric) error {
		m.kind = counter
		return nil
	}
}
