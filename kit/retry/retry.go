package retry

import (
	"context"
	"fmt"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
)

// An Operation is executing by Retry().
// The operation will be retried using a backoff policy if it returns an error.
type Operation func() error

// Retry the operation o until it does not return error or BackOff stops.
// o is guaranteed to be run at least once.
//
// If o returns an error wrapping ErrPermanent, the operation is not retried and the error is returned.
// When retries stop (backoff Stop or context done), the last operation error is returned
// wrapped with ErrPermanent or the context error.
//
// Retry sleeps the goroutine for the duration returned by BackOff after a
// failed operation returns.
func Retry(ctx context.Context, o Operation, b BackOff) error {
	return doRetry(ctx, o, b, nil)
}

func doRetry(ctx context.Context, operation Operation, b BackOff, t timer) error {
	if t == nil {
		t = &defaultTimer{}
	}

	defer func() {
		t.Stop()
	}()

	b.Reset()
	var err error
	var next time.Duration
	for {
		err = operation()
		if err == nil {
			return nil
		}

		if errors.Is(err, ErrPermanent) {
			return err
		}

		if next = b.NextBackOff(); next == Stop {
			// case of cancel error.
			if cerr := ctx.Err(); cerr != nil {
				return giveUp(err, cerr)
			}
			return giveUp(err, ErrPermanent)
		}

		t.Start(next)

		select {
		case <-ctx.Done():
			return giveUp(err, ctx.Err())
		case <-t.C():
		}
	}
}

// giveUp returns the operation error annotated with the reason to stop retrying,
// both errors are kept in the chain (errors.Is matches the operation error and the reason).
func giveUp(err error, reason error) error {
	return fmt.Errorf("%w: %w", err, reason)
}
