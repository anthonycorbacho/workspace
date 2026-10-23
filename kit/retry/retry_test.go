package retry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
)

type testTimer struct {
	timer *time.Timer
}

func (t *testTimer) Start(duration time.Duration) {
	t.timer = time.NewTimer(0)
}

func (t *testTimer) Stop() {
	if t.timer != nil {
		t.timer.Stop()
	}
}

func (t *testTimer) C() <-chan time.Time {
	return t.timer.C
}

func TestRetry(t *testing.T) {
	const successOn = 3
	i := 0

	// This function is successful on "successOn" calls.
	f := func() error {
		i++
		if i == successOn {
			return nil
		}
		return errors.New("error")
	}

	err := Retry(context.Background(), f, NewExponentialBackOff())
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if i != successOn {
		t.Errorf("invalid number of retries: %d", i)
	}
}

func TestRetryContext(t *testing.T) {
	cancelOn := 3
	i := 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This function cancels context on "cancelOn" calls.
	f := func() error {
		i++
		// cancelling the context in the operation function is not a typical
		// use-case, however it allows to get predictable test results.
		if i == cancelOn {
			cancel()
		}
		return fmt.Errorf("error (%d)", i)
	}

	err := Retry(ctx, f, NewConstantBackOff(time.Millisecond))
	if err == nil {
		t.Errorf("error is unexpectedly nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if i != cancelOn {
		t.Errorf("invalid number of retries: %d", i)
	}
}
