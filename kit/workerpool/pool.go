package workerpool

import "github.com/gammazero/workerpool"

// ErrStopped is returned by Submit when the worker pool is stopped.
var ErrStopped = workerpool.ErrStopped

// Task is the function task that will be submitted to the workers.
type Task func()

// Pool represents a worker pool that will execute tasks.
type Pool struct {
	wp *workerpool.WorkerPool
}

// New create a new Pool.
func New(maxWorkers int) *Pool {
	return &Pool{
		wp: workerpool.New(maxWorkers),
	}
}

// Submit enqueues a function for a worker to execute. Returns ErrStopped if the
// worker pool is stopped.
//
// Any external values needed by the task function must be captured in a
// closure.
// Any return values should be returned over a channel that is
// captured in the task function closure.
//
// Submit will not block regardless of the number of tasks submitted, and is
// safe to call concurrently with Stop or ForceStop.
func (p *Pool) Submit(task Task) error {
	return p.wp.Do(task)
}

// Stop stops the worker pool and waits for all queued tasks tasks to
// complete. No additional tasks may be submitted, but all pending tasks are
// executed by workers before this function returns.
func (p *Pool) Stop() {
	p.wp.StopWait()
}

// ForceStop stops the worker pool and waits for only currently running tasks to
// complete. Pending tasks that are not currently running are abandoned.
func (p *Pool) ForceStop() {
	p.wp.Stop()
}

// Stopped returns true if this worker pool has been stopped.
func (p *Pool) Stopped() bool {
	return p.wp.Stopped()
}
