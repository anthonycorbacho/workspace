package sql

import (
	stderrors "errors"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// meterProvider returns the meter provider reporting the database metrics, replaced in tests.
var meterProvider = otel.GetMeterProvider

// callbackRecorder is a metric.MeterProvider recording the callbacks registered by its meters.
//
// otelsql registers a DB stats callback that it never unregisters and does not expose its
// registration: the callback keeps the *sql.DB reachable and reporting after it is closed.
// DB.Close unregisters the recorded callbacks.
type callbackRecorder struct {
	metric.MeterProvider

	mu   sync.Mutex
	regs []metric.Registration
}

func newCallbackRecorder() *callbackRecorder {
	return &callbackRecorder{MeterProvider: meterProvider()}
}

// Meter returns a meter recording its callbacks registration.
func (r *callbackRecorder) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return &recordingMeter{Meter: r.MeterProvider.Meter(name, opts...), recorder: r}
}

func (r *callbackRecorder) record(reg metric.Registration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.regs = append(r.regs, reg)
}

// unregister unregisters all the recorded callbacks.
func (r *callbackRecorder) unregister() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	errs := make([]error, 0, len(r.regs))
	for _, reg := range r.regs {
		errs = append(errs, reg.Unregister())
	}
	r.regs = nil
	return stderrors.Join(errs...)
}

type recordingMeter struct {
	metric.Meter
	recorder *callbackRecorder
}

// RegisterCallback registers f and records its registration.
func (m *recordingMeter) RegisterCallback(f metric.Callback, instruments ...metric.Observable) (metric.Registration, error) {
	reg, err := m.Meter.RegisterCallback(f, instruments...)
	if err == nil {
		m.recorder.record(reg)
	}
	return reg, err
}
