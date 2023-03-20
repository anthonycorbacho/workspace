package dlock

import (
	"context"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
)

// DistributedLock provide a way to acquired distributed lock
// The lock should be unique across multiples go routine / multiples server.
//
//go:generate mockery --name DistributedLock --output mock --outpkg mock --with-expecter
type DistributedLock interface {
	// New return a lock for the value.
	// Nothing is lock until lock.Lock() is called.
	New(value string) (Lock, error)
}

// Lock the resource or release it.
// It should not be share across code (go routine) that want to lock the same resources.
// When the struct success lock, all the other call to lock will be successful until
// release or disconnected from the db
// You can use ContextLock to have a context that will be canceled when the lock was lost
// You can use WaitForLock to wait for lock until the context is cancel
//
//go:generate mockery --name Lock --output mock --outpkg mock --with-expecter
type Lock interface {
	// Lock ensure we lock the value if not already lock
	// if you are the owner of the lock it will do nothing
	// should be call before any important operation
	// You can use WaitForLock to wait until the context timeout
	// or lock get acquired
	Lock(ctx context.Context) error
	// IsLock will return nil only when it is lock
	IsLock(ctx context.Context) error
	// Release the lock and allow other to lock it
	Release() error
}

// ContextLock will return a new context that timeout when the lock was lost
// it return an error if the lock is not acquired
// It might have a little delay (time.Second) between lost lock and context cancel
func ContextLock(ctx context.Context, l Lock) (context.Context, context.CancelFunc, error) {
	err := l.Lock(ctx)
	if err != nil {
		return nil, nil, err
	}

	newCtx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-newCtx.Done():
				return
			case <-t.C:
				err := l.IsLock(newCtx)
				if err != nil {
					cancel()
					return
				}
				// normal case we continue to loop
			}
		}
	}()
	return newCtx, cancel, nil
}

// WaitForLock until ctx is timeout
// return nil when lock was acquired, or error when timeout (context timeout)
// It is not deterministic in the ordering of lock acquired
func WaitForLock(ctx context.Context, l Lock) error {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "timeout to acquired lock")
		case <-t.C:
			err := l.Lock(ctx)
			if err != nil {
				continue
			}
			return nil
		}
	}
}
