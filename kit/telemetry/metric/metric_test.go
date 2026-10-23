package metric

import (
	"testing"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collect returns the metrics exposed by the collector.
func collect(t *testing.T, c prom.Collector) []*dto.Metric {
	t.Helper()
	ch := make(chan prom.Metric, 10)
	c.Collect(ch)
	close(ch)
	var metrics []*dto.Metric
	for m := range ch {
		d := &dto.Metric{}
		require.NoError(t, m.Write(d))
		metrics = append(metrics, d)
	}
	return metrics
}

func TestNewMetric(t *testing.T) {
	tests := []struct {
		name     string
		opts     []Option
		wantKind int
		wantErr  bool
	}{
		{name: "default is counter", wantKind: counter},
		{name: "counter", opts: []Option{Counter()}, wantKind: counter},
		{name: "gauge", opts: []Option{Gauge()}, wantKind: gauge},
		{name: "histogram", opts: []Option{Histogram(1, 2, 3)}, wantKind: histogram},
		{name: "histogram unsorted buckets", opts: []Option{Histogram(3, 1, 2)}, wantKind: histogram},
		{name: "summary", opts: []Option{Summary(map[float64]float64{0.5: 0.05})}, wantKind: summary},
		{name: "summary with max age", opts: []Option{Summary(nil), MaxAge(time.Minute)}, wantKind: summary},
		{name: "last kind wins", opts: []Option{Gauge(), Counter()}, wantKind: counter},
		{name: "with labels", opts: []Option{Gauge(), Labels("a", "b")}, wantKind: gauge},
		{name: "empty buckets", opts: []Option{Histogram()}, wantErr: true},
		{name: "duplicated buckets", opts: []Option{Histogram(1, 2, 2)}, wantErr: true},
		{name: "zero max age", opts: []Option{Summary(nil), MaxAge(0)}, wantErr: true},
		{name: "negative max age", opts: []Option{MaxAge(-time.Second)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newMetric(uniqueName("kit_test_new_metric"), "help", tt.opts...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, m)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, m.kind)
			assert.NotNil(t, m.Collector())
		})
	}
}

func TestOptions(t *testing.T) {
	m, err := newMetric(uniqueName("kit_test_options"), "help",
		Labels("a"),
		Labels("b", "c"),
		Histogram(5, 1, 3),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, m.labels)
	assert.Equal(t, []float64{1, 3, 5}, m.buckets, "buckets are sorted")

	objectives := map[float64]float64{0.5: 0.05, 0.9: 0.01}
	m, err = newMetric(uniqueName("kit_test_options"), "help", Summary(objectives), MaxAge(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, objectives, m.objectives)
	assert.Equal(t, time.Minute, m.maxAge)

	m, err = newMetric(uniqueName("kit_test_options"), "help", Summary(nil))
	require.NoError(t, err)
	assert.Equal(t, prom.DefMaxAge, m.maxAge, "default max age")
}

func TestMetricCollector(t *testing.T) {
	tests := []struct {
		name string
		kind int
		want any
	}{
		{name: "counter", kind: counter, want: &prom.CounterVec{}},
		{name: "gauge", kind: gauge, want: &prom.GaugeVec{}},
		{name: "histogram", kind: histogram, want: &prom.HistogramVec{}},
		{name: "summary", kind: summary, want: &prom.SummaryVec{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newMetric(uniqueName("kit_test_collector"), "help", func(m *metric) error {
				m.kind = tt.kind
				m.buckets = []float64{1}
				return nil
			})
			require.NoError(t, err)
			assert.IsType(t, tt.want, m.Collector())
		})
	}

	t.Run("unknown kind", func(t *testing.T) {
		m := &metric{kind: 42}
		assert.Nil(t, m.Collector())
	})
}

func TestMetricAdd(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		values  []float64
		labels  []string
		want    float64
		wantErr bool
	}{
		{name: "counter", opts: []Option{Counter()}, values: []float64{1, 2.5}, want: 3.5},
		{name: "counter with labels", opts: []Option{Counter(), Labels("code")}, values: []float64{1, 1}, labels: []string{"200"}, want: 2},
		{name: "counter zero", opts: []Option{Counter()}, values: []float64{0}, want: 0},
		{name: "counter negative", opts: []Option{Counter()}, values: []float64{-1}, wantErr: true},
		{name: "counter missing label", opts: []Option{Counter(), Labels("code")}, values: []float64{1}, wantErr: true},
		{name: "counter extra label", opts: []Option{Counter()}, values: []float64{1}, labels: []string{"200"}, wantErr: true},
		{name: "gauge", opts: []Option{Gauge()}, values: []float64{5, -2}, want: 3},
		{name: "gauge with labels", opts: []Option{Gauge(), Labels("pool")}, values: []float64{-1}, labels: []string{"db"}, want: -1},
		{name: "gauge missing label", opts: []Option{Gauge(), Labels("pool")}, values: []float64{1}, wantErr: true},
		{name: "histogram", opts: []Option{Histogram(1)}, values: []float64{1}, wantErr: true},
		{name: "summary", opts: []Option{Summary(nil)}, values: []float64{1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newMetric(uniqueName("kit_test_add"), "help", tt.opts...)
			require.NoError(t, err)

			for _, v := range tt.values {
				err = m.Add(v, tt.labels...)
				if tt.wantErr {
					assert.Error(t, err)
					return
				}
				require.NoError(t, err)
			}

			metrics := collect(t, m.Collector())
			require.Len(t, metrics, 1)
			if m.kind == counter {
				assert.Equal(t, tt.want, metrics[0].GetCounter().GetValue())
			} else {
				assert.Equal(t, tt.want, metrics[0].GetGauge().GetValue())
			}
			assert.Len(t, metrics[0].GetLabel(), len(tt.labels))
		})
	}
}

func TestMetricSet(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		values  []float64
		labels  []string
		want    float64
		wantErr bool
	}{
		{name: "gauge", opts: []Option{Gauge()}, values: []float64{5, 2}, want: 2},
		{name: "gauge negative", opts: []Option{Gauge()}, values: []float64{-5}, want: -5},
		{name: "gauge with labels", opts: []Option{Gauge(), Labels("pool")}, values: []float64{7}, labels: []string{"db"}, want: 7},
		{name: "gauge missing label", opts: []Option{Gauge(), Labels("pool")}, values: []float64{1}, wantErr: true},
		{name: "counter", opts: []Option{Counter()}, values: []float64{1}, wantErr: true},
		{name: "histogram", opts: []Option{Histogram(1)}, values: []float64{1}, wantErr: true},
		{name: "summary", opts: []Option{Summary(nil)}, values: []float64{1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newMetric(uniqueName("kit_test_set"), "help", tt.opts...)
			require.NoError(t, err)

			for _, v := range tt.values {
				err = m.Set(v, tt.labels...)
				if tt.wantErr {
					assert.Error(t, err)
					return
				}
				require.NoError(t, err)
			}

			metrics := collect(t, m.Collector())
			require.Len(t, metrics, 1)
			assert.Equal(t, tt.want, metrics[0].GetGauge().GetValue())
		})
	}
}

func TestMetricObserve(t *testing.T) {
	tests := []struct {
		name      string
		opts      []Option
		values    []float64
		labels    []string
		wantCount uint64
		wantSum   float64
		wantErr   bool
	}{
		{name: "histogram", opts: []Option{Histogram(1, 5)}, values: []float64{0.5, 3, 10}, wantCount: 3, wantSum: 13.5},
		{name: "histogram with labels", opts: []Option{Histogram(1), Labels("route")}, values: []float64{2}, labels: []string{"/"}, wantCount: 1, wantSum: 2},
		{name: "histogram missing label", opts: []Option{Histogram(1), Labels("route")}, values: []float64{1}, wantErr: true},
		{name: "summary", opts: []Option{Summary(map[float64]float64{0.5: 0.05})}, values: []float64{1, 2}, wantCount: 2, wantSum: 3},
		{name: "summary with labels", opts: []Option{Summary(nil), Labels("route")}, values: []float64{4}, labels: []string{"/"}, wantCount: 1, wantSum: 4},
		{name: "summary missing label", opts: []Option{Summary(nil), Labels("route")}, values: []float64{1}, wantErr: true},
		{name: "gauge sets the value", opts: []Option{Gauge()}, values: []float64{4, 2}, wantSum: 2},
		{name: "gauge missing label", opts: []Option{Gauge(), Labels("pool")}, values: []float64{1}, wantErr: true},
		{name: "counter", opts: []Option{Counter()}, values: []float64{1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newMetric(uniqueName("kit_test_observe"), "help", tt.opts...)
			require.NoError(t, err)

			for _, v := range tt.values {
				err = m.Observe(v, tt.labels...)
				if tt.wantErr {
					assert.Error(t, err)
					return
				}
				require.NoError(t, err)
			}

			metrics := collect(t, m.Collector())
			require.Len(t, metrics, 1)
			switch m.kind {
			case histogram:
				assert.Equal(t, tt.wantCount, metrics[0].GetHistogram().GetSampleCount())
				assert.Equal(t, tt.wantSum, metrics[0].GetHistogram().GetSampleSum())
			case summary:
				assert.Equal(t, tt.wantCount, metrics[0].GetSummary().GetSampleCount())
				assert.Equal(t, tt.wantSum, metrics[0].GetSummary().GetSampleSum())
			case gauge:
				assert.Equal(t, tt.wantSum, metrics[0].GetGauge().GetValue())
			}
		})
	}
}

func TestMetricUnknownKind(t *testing.T) {
	m := &metric{kind: 42}
	assert.Error(t, m.Add(1))
	assert.Error(t, m.Set(1))
	assert.Error(t, m.Observe(1))
}
