package workerpool

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool(t *testing.T) {
	tests := []struct {
		name       string
		maxWorkers int
		tasks      int
		wantLimit  int
	}{
		{name: "single worker", maxWorkers: 1, tasks: 20, wantLimit: 1},
		{name: "multiple workers", maxWorkers: 4, tasks: 50, wantLimit: 4},
		{name: "more workers than tasks", maxWorkers: 10, tasks: 3, wantLimit: 10},
		{name: "no task", maxWorkers: 2, tasks: 0, wantLimit: 2},
		{name: "zero workers defaults to one", maxWorkers: 0, tasks: 10, wantLimit: 1},
		{name: "negative workers defaults to one", maxWorkers: -1, tasks: 10, wantLimit: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.maxWorkers)
			require.NotNil(t, p)
			assert.False(t, p.Stopped())

			var running, maxRunning, done atomic.Int64
			for range tt.tasks {
				require.NoError(t, p.Submit(func() {
					cur := running.Add(1)
					for {
						m := maxRunning.Load()
						if cur <= m || maxRunning.CompareAndSwap(m, cur) {
							break
						}
					}
					time.Sleep(time.Millisecond)
					running.Add(-1)
					done.Add(1)
				}))
			}

			// Stop waits for all the queued tasks to complete.
			p.Stop()
			assert.True(t, p.Stopped())
			assert.Equal(t, int64(tt.tasks), done.Load())
			assert.LessOrEqual(t, maxRunning.Load(), int64(tt.wantLimit))
		})
	}
}

func TestPoolSubmitNil(t *testing.T) {
	p := New(1)
	assert.NoError(t, p.Submit(nil))
	p.Stop()
	assert.True(t, p.Stopped())
}

func TestPoolSubmitAfterStop(t *testing.T) {
	p := New(1)
	p.Stop()
	assert.ErrorIs(t, p.Submit(func() {}), ErrStopped)
}

func TestPoolSubmitConcurrentWithStop(t *testing.T) {
	p := New(2)

	var wg sync.WaitGroup
	var accepted, rejected, executed atomic.Int64

	const producers = 8
	const perProducer = 100
	start := make(chan struct{})
	for range producers {
		wg.Go(func() {
			<-start
			for range perProducer {
				err := p.Submit(func() {
					executed.Add(1)
				})
				switch {
				case err == nil:
					accepted.Add(1)
				case errors.Is(err, ErrStopped):
					rejected.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}

	close(start)
	time.Sleep(time.Millisecond)
	p.Stop()
	wg.Wait()

	assert.True(t, p.Stopped())
	assert.Equal(t, int64(producers*perProducer), accepted.Load()+rejected.Load())
	assert.Equal(t, accepted.Load(), executed.Load())
}

func TestPoolStopIdempotent(t *testing.T) {
	p := New(2)
	assert.NotPanics(t, func() {
		p.Stop()
		p.Stop()
		p.ForceStop()
	})
	assert.True(t, p.Stopped())
}

func TestPoolForceStop(t *testing.T) {
	const pending = 5

	p := New(1)

	started := make(chan struct{})
	release := make(chan struct{})
	require.NoError(t, p.Submit(func() {
		close(started)
		<-release
	}))
	<-started

	// the only worker is busy, those tasks are queued.
	var done atomic.Int64
	for range pending {
		require.NoError(t, p.Submit(func() { done.Add(1) }))
	}

	stopped := make(chan struct{})
	go func() {
		p.ForceStop()
		close(stopped)
	}()

	// ForceStop waits for the running task to complete.
	require.Eventually(t, p.Stopped, 5*time.Second, time.Millisecond)
	select {
	case <-stopped:
		t.Fatal("ForceStop returned before the running task completed")
	default:
	}
	close(release)

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("ForceStop did not return")
	}
	// pending tasks are abandoned.
	assert.Less(t, done.Load(), int64(pending))
}
