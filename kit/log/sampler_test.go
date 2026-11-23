package log

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSamplingHandler(t *testing.T) {
	out := &syncBuffer{}
	h := newSamplingHandler(newJSONHandler(out, DebugLevel), time.Second, 2, 3)

	base := time.Unix(1_700_000_000, 0)
	ctx := context.Background()

	// Send 8 identical messages within one tick:
	// allowed: #1, #2 (initial=2), #5 (2+3), #8 (2+6) => 4 entries.
	for range 8 {
		rec := slog.NewRecord(base, slog.LevelInfo, "noisy", 0)
		require.NoError(t, h.Handle(ctx, rec))
	}
	assert.Len(t, out.entries(t), 4)

	// Next tick resets the counter and allows another initial burst.
	for range 2 {
		rec := slog.NewRecord(base.Add(time.Second), slog.LevelInfo, "noisy", 0)
		require.NoError(t, h.Handle(ctx, rec))
	}
	assert.Len(t, out.entries(t), 6)

	// Fatal entries are never sampled out.
	for range 5 {
		rec := slog.NewRecord(base.Add(time.Second), FatalLevel.slogLevel(), "fatal", 0)
		require.NoError(t, h.Handle(ctx, rec))
	}
	assert.Len(t, out.entries(t), 11)
}
