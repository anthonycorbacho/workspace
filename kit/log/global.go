package log

import "sync"

var (
	_globalMu sync.RWMutex
	_globalL  = NewNop()
)

// L returns the global Logger, which can be reconfigured with ReplaceGlobal.
// By default, global logger is set to no-op.
// It's safe for concurrent use.
func L() *Logger {
	_globalMu.RLock()
	l := _globalL
	_globalMu.RUnlock()
	return l
}

// ReplaceGlobal replaces the global Logger and returns a
// function to restore the original values.
// It's safe for concurrent use.
func ReplaceGlobal(logger *Logger) func() {
	_globalMu.Lock()
	prev := _globalL
	_globalL = logger
	_globalMu.Unlock()
	return func() {
		ReplaceGlobal(prev)
	}
}
