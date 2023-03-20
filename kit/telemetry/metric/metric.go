package metric

import (
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	prom "github.com/prometheus/client_golang/prometheus"
)

// Metric types.
const (
	histogram = iota + 1
	summary
	gauge
	counter
)

// metric is used to collect telemetry for a named operation, optionally broken down
// by multiple labels.
type metric struct {
	Name       string
	Help       string
	kind       int
	labels     []string
	buckets    []float64
	objectives map[float64]float64
	maxAge     time.Duration

	histogramVec *prom.HistogramVec
	summaryVec   *prom.SummaryVec
	gaugeVec     *prom.GaugeVec
	counterVec   *prom.CounterVec
}

// newMetric creates a new metric from the given options.
func newMetric(name, help string, options ...Option) (*metric, error) {
	// Apply options
	m := &metric{
		Name:       name,
		Help:       help,
		kind:       counter,
		labels:     []string{},
		buckets:    []float64{},
		objectives: map[float64]float64{},
		maxAge:     prom.DefMaxAge,
	}
	for _, opt := range options {
		err := opt(m)
		if err != nil {
			return nil, err
		}
	}

	switch m.kind {
	case histogram:
		m.histogramVec = prom.NewHistogramVec(prom.HistogramOpts{
			Name:    m.Name,
			Help:    m.Help,
			Buckets: m.buckets,
		}, m.labels)

	case summary:
		m.summaryVec = prom.NewSummaryVec(prom.SummaryOpts{
			Name:       m.Name,
			Help:       m.Help,
			Objectives: m.objectives,
			MaxAge:     m.maxAge,
		}, m.labels)

	case gauge:
		m.gaugeVec = prom.NewGaugeVec(prom.GaugeOpts{
			Name: m.Name,
			Help: m.Help,
		}, m.labels)

	case counter:
		m.counterVec = prom.NewCounterVec(prom.CounterOpts{
			Name: m.Name,
			Help: m.Help,
		}, m.labels)
	}

	return m, nil
}

// Add the given value to a counter or gauge metric.
// An error will be returned if a negative value is added to a counter.
func (m *metric) Add(val float64, labels ...string) error {

	switch m.kind {
	case counter:
		counter, err := m.counterVec.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		if val < 0 {
			return errors.New("value must not be negative")
		}
		counter.Add(val)
		return nil

	case gauge:
		gauge, err := m.gaugeVec.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		gauge.Add(val)
		return nil

	default:
		return errors.New("unsupported operation")

	}

}

// Set the given value to a gauge metric.
func (m *metric) Set(val float64, labels ...string) error {

	switch m.kind {
	case gauge:
		gauge, err := m.gaugeVec.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		gauge.Set(val)
		return nil

	default:
		return errors.New("unsupported operation")

	}

}

// Observe the given value using a histogram or summary, or set it as a gauge's value.
func (m *metric) Observe(val float64, labels ...string) error {

	switch m.kind {
	case histogram:
		histogram, err := m.histogramVec.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		histogram.Observe(val)
		return nil
	case summary:
		summary, err := m.summaryVec.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		summary.Observe(val)
		return nil
	case gauge:
		gauge, err := m.gaugeVec.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		gauge.Set(val)
		return nil

	default:
		return errors.New("unsupported operation")

	}
}

// Collector is the Prometheus interface of the metric used to register it.
func (m *metric) Collector() prom.Collector {
	switch m.kind {
	case histogram:
		return m.histogramVec
	case gauge:
		return m.gaugeVec
	case counter:
		return m.counterVec
	case summary:
		return m.summaryVec

	default:
		return nil
	}
}
