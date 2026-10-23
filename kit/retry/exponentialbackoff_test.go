package retry

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackOff(t *testing.T) {
	var (
		testInitialInterval     = 500 * time.Millisecond
		testRandomizationFactor = 0.1
		testMultiplier          = 2.0
		testMaxInterval         = 5 * time.Second
		testMaxElapsedTime      = 15 * time.Minute
	)

	exp := NewExponentialBackOff()
	exp.InitialInterval = testInitialInterval
	exp.RandomizationFactor = testRandomizationFactor
	exp.Multiplier = testMultiplier
	exp.MaxInterval = testMaxInterval
	exp.MaxElapsedTime = testMaxElapsedTime
	exp.Reset()

	expectedResults := []time.Duration{500, 1000, 2000, 4000, 5000, 5000, 5000, 5000, 5000, 5000}
	for i, d := range expectedResults {
		expectedResults[i] = d * time.Millisecond
	}

	for _, expected := range expectedResults {
		assert.Equal(t, expected, exp.currentInterval)
		// Assert that the next backoff falls in the expected range.
		minInterval := expected - time.Duration(testRandomizationFactor*float64(expected))
		maxInterval := expected + time.Duration(testRandomizationFactor*float64(expected))
		actualInterval := exp.NextBackOff()
		if !(minInterval <= actualInterval && actualInterval <= maxInterval) {
			t.Error("error")
		}
	}
}

func TestGetRandomizedInterval(t *testing.T) {
	// 33% chance of being 1.
	assert.Equal(t, time.Duration(1), getRandomValueFromInterval(0.5, 0, 2))
	assert.Equal(t, time.Duration(1), getRandomValueFromInterval(0.5, 0.33, 2))
	// 33% chance of being 2.
	assert.Equal(t, time.Duration(2), getRandomValueFromInterval(0.5, 0.34, 2))
	assert.Equal(t, time.Duration(2), getRandomValueFromInterval(0.5, 0.66, 2))
	// 33% chance of being 3.
	assert.Equal(t, time.Duration(3), getRandomValueFromInterval(0.5, 0.67, 2))
	assert.Equal(t, time.Duration(3), getRandomValueFromInterval(0.5, 0.99, 2))
}

type TestClock struct {
	i     time.Duration
	start time.Time
}

func (c *TestClock) Now() time.Time {
	t := c.start.Add(c.i)
	c.i += time.Second
	return t
}

func TestGetElapsedTime(t *testing.T) {
	exp := NewExponentialBackOff()
	exp.Clock = &TestClock{}
	exp.Reset()

	elapsedTime := exp.GetElapsedTime()
	if elapsedTime != time.Second {
		t.Errorf("elapsedTime=%d", elapsedTime)
	}
}

func TestMaxElapsedTime(t *testing.T) {
	exp := NewExponentialBackOff()
	exp.Clock = &TestClock{start: time.Time{}.Add(10000 * time.Second)}
	// Change the currentElapsedTime to be 0 ensuring that the elapsed time will be greater
	// than the max elapsed time.
	exp.startTime = time.Time{}
	assert.Equal(t, Stop, exp.NextBackOff())
}

func TestCustomStop(t *testing.T) {
	exp := NewExponentialBackOff()
	customStop := time.Minute
	exp.Stop = customStop
	exp.Clock = &TestClock{start: time.Time{}.Add(10000 * time.Second)}
	// Change the currentElapsedTime to be 0 ensuring that the elapsed time will be greater
	// than the max elapsed time.
	exp.startTime = time.Time{}
	assert.Equal(t, customStop, exp.NextBackOff())
}

func TestBackOffOverflow(t *testing.T) {
	var (
		testInitialInterval time.Duration = math.MaxInt64 / 2
		testMaxInterval     time.Duration = math.MaxInt64
		testMultiplier                    = 2.1
	)

	exp := NewExponentialBackOff()
	exp.InitialInterval = testInitialInterval
	exp.Multiplier = testMultiplier
	exp.MaxInterval = testMaxInterval
	exp.Reset()

	exp.NextBackOff()
	// Assert that when an overflow is possible, the current interval time.Duration is set to the max interval time.Duration.
	assert.Equal(t, testMaxInterval, exp.currentInterval)
}

func TestNewExponentialBackOff(t *testing.T) {
	// Create a new ExponentialBackOff with custom options
	backOff := NewExponentialBackOff(
		WithInitialInterval(1*time.Second),
		WithMultiplier(2.0),
		WithMaxInterval(10*time.Second),
		WithMaxElapsedTime(30*time.Second),
		WithRetryStopDuration(0),
		WithClockProvider(SystemClock),
	)

	// Check that the backOff object is not nil
	if backOff == nil {
		t.Error("Expected a non-nil ExponentialBackOff object, got nil")
	}

	// Check that the custom options were applied correctly
	if backOff.InitialInterval != 1*time.Second {
		t.Errorf("Expected InitialInterval to be 1 second, got %v", backOff.InitialInterval)
	}

	if backOff.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier to be 2.0, got %v", backOff.Multiplier)
	}

	if backOff.MaxInterval != 10*time.Second {
		t.Errorf("Expected MaxInterval to be 10 seconds, got %v", backOff.MaxInterval)
	}

	if backOff.MaxElapsedTime != 30*time.Second {
		t.Errorf("Expected MaxElapsedTime to be 30 seconds, got %v", backOff.MaxElapsedTime)
	}

	if backOff.Stop != 0 {
		t.Errorf("Expected Stop to be 0 (no stop), got %v", backOff.Stop)
	}

	if backOff.Clock != SystemClock {
		t.Errorf("Expected Clock to be SystemClock, got %v", backOff.Clock)
	}
}
