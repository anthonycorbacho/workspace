package metric

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/id"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uniqueName returns a metric name unique to the test run, metrics are registered
// in the global prometheus registry and tests can run several times (eg: -count, -cpu).
func uniqueName(name string) string {
	return fmt.Sprintf("%s_%s", name, id.New())
}

func TestMetricsConcurrentRegisterAndObserve(t *testing.T) {
	m := New()
	observed := uniqueName("kit_test_observed")
	require.NoError(t, m.Register(observed, "test histogram", Histogram(1, 2, 3)))

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			assert.NoError(t, m.Register(uniqueName(fmt.Sprintf("kit_test_gauge_%d", i)), "test gauge", Gauge()))
		})
		wg.Go(func() {
			assert.NoError(t, m.Observe(observed, float64(i)))
		})
	}
	wg.Wait()
}

func TestMetricsRegister(t *testing.T) {
	m := New()
	counter := uniqueName("kit_test_counter")
	require.NoError(t, m.Register(counter, "test counter", Counter()))

	assert.Error(t, m.Register(counter, "test counter", Counter()), "duplicated metric name")
	assert.Error(t, m.Increment("unknown", 1))
	assert.Error(t, m.Observe("unknown", 1))
	assert.NoError(t, m.Increment(counter, 1))
}

func TestMetricsRegisterFailureIsNotKept(t *testing.T) {
	first, second := New(), New()
	conflict := uniqueName("kit_test_conflict")
	require.NoError(t, first.Register(conflict, "test counter", Counter()))

	// the name is already registered in the prometheus registry by another Metrics.
	require.Error(t, second.Register(conflict, "test counter", Counter()))
	assert.Error(t, second.Increment(conflict, 1))
}

func TestMetricsRegisterErrors(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
	}{
		{name: "invalid option", opts: []Option{Histogram()}},
		{name: "invalid max age", opts: []Option{Summary(nil), MaxAge(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			name := uniqueName("kit_test_register_error")
			require.Error(t, m.Register(name, "help", tt.opts...))

			// the failed metric is not kept and the name can be registered again.
			assert.Error(t, m.Increment(name, 1))
			assert.NoError(t, m.Register(name, "help", Counter()))
		})
	}

	t.Run("empty prometheus name", func(t *testing.T) {
		m := New()
		assert.Error(t, m.Register("", "help", Counter()))
		assert.Error(t, m.Increment("", 1))
	})

	t.Run("reserved label name", func(t *testing.T) {
		m := New()
		name := uniqueName("kit_test_reserved_label")
		assert.Error(t, m.Register(name, "help", Counter(), Labels("__reserved")))
		assert.Error(t, m.Increment(name, 1, "value"))
	})
}

func TestMetricsOperations(t *testing.T) {
	tests := []struct {
		name         string
		opts         []Option
		labels       []string
		incrementErr bool
		setErr       bool
		observeErr   bool
	}{
		{name: "counter", opts: []Option{Counter()}, setErr: true, observeErr: true},
		{name: "counter with labels", opts: []Option{Counter(), Labels("code")}, labels: []string{"200"}, setErr: true, observeErr: true},
		{name: "gauge", opts: []Option{Gauge()}},
		{name: "gauge with labels", opts: []Option{Gauge(), Labels("pool", "kind")}, labels: []string{"db", "read"}},
		{name: "histogram", opts: []Option{Histogram(0.1, 1)}, incrementErr: true, setErr: true},
		{name: "summary", opts: []Option{Summary(map[float64]float64{0.9: 0.01}), MaxAge(time.Minute)}, incrementErr: true, setErr: true},
		{name: "labels mismatch", opts: []Option{Gauge(), Labels("pool")}, incrementErr: true, setErr: true, observeErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			name := uniqueName("kit_test_operations")
			require.NoError(t, m.Register(name, "help", tt.opts...))

			assert.Equal(t, tt.incrementErr, m.Increment(name, 1, tt.labels...) != nil, "increment")
			assert.Equal(t, tt.setErr, m.Set(name, 1, tt.labels...) != nil, "set")
			assert.Equal(t, tt.observeErr, m.Observe(name, 1, tt.labels...) != nil, "observe")
		})
	}
}

func TestMetricsUnknown(t *testing.T) {
	m := New()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "increment", call: func() error { return m.Increment("unknown", 1) }},
		{name: "set", call: func() error { return m.Set("unknown", 1) }},
		{name: "observe", call: func() error { return m.Observe("unknown", 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown metric 'unknown'")
		})
	}
}

func TestMetricsGathered(t *testing.T) {
	m := New()
	name := uniqueName("kit_test_gathered")
	require.NoError(t, m.Register(name, "test gauge", Gauge(), Labels("pool")))
	require.NoError(t, m.Set(name, 42, "db"))

	families, err := prom.DefaultGatherer.Gather()
	require.NoError(t, err)
	var found bool
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		found = true
		assert.Equal(t, "test gauge", f.GetHelp())
		require.Len(t, f.GetMetric(), 1)
		assert.Equal(t, float64(42), f.GetMetric()[0].GetGauge().GetValue())
		assert.Equal(t, "db", f.GetMetric()[0].GetLabel()[0].GetValue())
	}
	assert.True(t, found, "metric must be registered in the default registry")
}
