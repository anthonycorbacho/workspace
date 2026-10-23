package retry

import (
	"testing"
	"time"
)

func TestConstantBackOff(t *testing.T) {
	backoff := NewConstantBackOff(time.Second)
	if backoff.NextBackOff() != time.Second {
		t.Error("invalid interval")
	}
}
