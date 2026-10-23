// Package workerpool concurrency limiting goroutine pool.
// Limits the concurrency of task execution, not the number of tasks queued.
// Never blocks submitting tasks, no matter how many tasks are queued.
package workerpool
