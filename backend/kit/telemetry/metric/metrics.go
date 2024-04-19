package metric

import (
	"sync"

	"github.com/anthonycorbacho/workspace/kit/errors"
	prom "github.com/prometheus/client_golang/prometheus"
)

// Metrics defines a set of metric collection.
type Metrics struct {
	metricLock sync.RWMutex
	metrics    map[string]*metric
}

// New create a new metrics.
func New() *Metrics {
	return &Metrics{
		metrics: make(map[string]*metric),
	}
}

// Register register a new metric.
func (m *Metrics) Register(name, help string, opts ...Option) error {
	m.metricLock.Lock()
	defer m.metricLock.Unlock()
	if _, ok := m.metrics[name]; ok {
		return errors.New("metric already defined")
	}

	mtr, err := newMetric(name, help, opts...)
	if err != nil {
		return err
	}
	m.metrics[name] = mtr

	return prom.Register(mtr.Collector())
}

// Increment adds the given value to a counter or gauge metric.
// The name and labels must match a previously defined metric.
// Gauge metrics support subtraction by use of a negative value.
// Counter metrics only allow addition and a negative value will result in an error.
func (m *Metrics) Increment(name string, val float64, labels ...string) error {
	m.metricLock.RLock()
	defer m.metricLock.RUnlock()
	mtr, ok := m.metrics[name]
	if !ok {
		return errors.Newf("unknown metric '%s'", name)
	}
	return mtr.Add(val, labels...)

}

// Set replace the given value to a gauge metric.
// The name and labels must match a previously defined metric.
func (m *Metrics) Set(name string, val float64, labels ...string) error {
	m.metricLock.RLock()
	defer m.metricLock.RUnlock()
	mtr, ok := m.metrics[name]
	if !ok {
		return errors.Newf("unknown metric '%s'", name)
	}
	return mtr.Set(val, labels...)

}

// Observe observes the given value using a histogram or summary, or sets it as a gauge's value.
// The name and labels must match a previously defined metric.
func (m *Metrics) Observe(name string, val float64, labels ...string) error {
	mtr, ok := m.metrics[name]
	if !ok {
		return errors.Newf("unknown metric '%s'", name)
	}

	return mtr.Observe(val, labels...)
}
