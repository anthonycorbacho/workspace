package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConstantBackOff(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{name: "one second", interval: time.Second},
		{name: "zero", interval: 0},
		{name: "stop", interval: Stop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := NewConstantBackOff(tt.interval)
			for range 3 {
				assert.Equal(t, tt.interval, backoff.NextBackOff())
			}
			backoff.Reset()
			assert.Equal(t, tt.interval, backoff.NextBackOff())
		})
	}
}
