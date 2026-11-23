package log

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sync/atomic"
	"time"
)

const samplerBuckets = 4096

type samplerCounter struct {
	resetAt atomic.Int64
	counter atomic.Uint64
}

type samplerState struct {
	tick       time.Duration
	initial    uint64
	thereafter uint64
	buckets    [samplerBuckets]samplerCounter
}

func (s *samplerState) allow(t time.Time, level slog.Level, msg string) bool {
	if level >= FatalLevel.slogLevel() {
		return true
	}

	b := &s.buckets[bucketIndex(level, msg)]
	now := t.UnixNano()
	resetAt := b.resetAt.Load()
	if now >= resetAt {
		nextReset := now + s.tick.Nanoseconds()
		if b.resetAt.CompareAndSwap(resetAt, nextReset) {
			b.counter.Store(0)
		}
	}

	n := b.counter.Add(1)
	if n <= s.initial {
		return true
	}
	if s.thereafter == 0 {
		return false
	}
	return (n-s.initial)%s.thereafter == 0
}

func bucketIndex(level slog.Level, msg string) uint32 {
	h := fnv.New32a()
	// hash.Hash.Write never returns an error.
	_, _ = h.Write([]byte{byte(level)}) //nolint:errcheck // fnv32a Write never errors.
	_, _ = h.Write([]byte(msg))         //nolint:errcheck // fnv32a Write never errors.
	return h.Sum32() & (samplerBuckets - 1)
}

// samplingHandler wraps a slog.Handler and limits repeated entries with the same level and message
// to the first initial entries per tick, and then 1 of every thereafter entries.
type samplingHandler struct {
	next  slog.Handler
	state *samplerState
}

var _ slog.Handler = (*samplingHandler)(nil)

func newSamplingHandler(next slog.Handler, tick time.Duration, initial, thereafter uint64) *samplingHandler {
	return &samplingHandler{
		next: next,
		state: &samplerState{
			tick:       tick,
			initial:    initial,
			thereafter: thereafter,
		},
	}
}

func (h *samplingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *samplingHandler) Handle(ctx context.Context, r slog.Record) error {
	if !h.state.allow(r.Time, r.Level, r.Message) {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *samplingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &samplingHandler{next: h.next.WithAttrs(attrs), state: h.state}
}

func (h *samplingHandler) WithGroup(name string) slog.Handler {
	return &samplingHandler{next: h.next.WithGroup(name), state: h.state}
}
