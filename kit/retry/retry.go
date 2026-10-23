package retry

import (
	"context"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
)

// An Operation is executing by Retry().
// The operation will be retried using a backoff policy if it returns an error.
type Operation func() error

// Retry the operation o until it does not return error or BackOff stops.
// o is guaranteed to be run at least once.
//
// If o returns a PermanentError, the operation is not retried, and the
// wrapped error is returned.
//
// Retry sleeps the goroutine for the duration returned by BackOff after a
// failed operation returns.
func Retry(ctx context.Context, o Operation, b BackOff) error {
	return doRetry(ctx, o, b, nil)
}

func doRetry(ctx context.Context, operation Operation, b BackOff, t Timer) error {
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
				return errors.Wrap(cerr, err.Error())
			}
			return errors.Wrap(ErrPermanent, err.Error())
		}

		t.Start(next)

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), err.Error())
		case <-t.C():
		}
	}
}
