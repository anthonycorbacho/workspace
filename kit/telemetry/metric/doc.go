// Package metric registers and records prometheus metrics by name, the metrics are registered in the default
// prometheus registry.
//
//	m := metric.New()
//	err := m.Register("jobs_processed_total", "Number of processed jobs.", metric.Counter(), metric.Labels("status"))
//	...
//	err = m.Increment("jobs_processed_total", 1, "success")
package metric
