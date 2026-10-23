package gcp

import (
	"testing"
	"time"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"github.com/stretchr/testify/assert"
)

func TestSubscriberOption(t *testing.T) {

	// Dummy
	c := gcppubsub.Client{}
	s, err := NewSubscriber(&c,
		WithNumGoroutines(1),
		WithMaxOutstandingMessages(42),
		WithMaxExtensionPeriod(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, gcppubsub.ReceiveSettings{
		MaxExtension:               60 * time.Minute,
		MaxDurationPerAckExtension: time.Minute,
		MinDurationPerAckExtension: 0,
		MaxOutstandingMessages:     42,
		MaxOutstandingBytes:        1e9,
		NumGoroutines:              1,
	}, s.settings)
}

func TestSubscriberOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  SubscriberOption
		want gcppubsub.ReceiveSettings
	}{
		{name: "max extension", opt: WithMaxExtension(time.Hour), want: gcppubsub.ReceiveSettings{MaxExtension: time.Hour}},
		{name: "max extension period", opt: WithMaxExtensionPeriod(time.Minute), want: gcppubsub.ReceiveSettings{MaxDurationPerAckExtension: time.Minute}},
		{name: "min extension period", opt: WithMinExtensionPeriod(time.Minute), want: gcppubsub.ReceiveSettings{MinDurationPerAckExtension: time.Minute}},
		{name: "max outstanding messages", opt: WithMaxOutstandingMessages(10), want: gcppubsub.ReceiveSettings{MaxOutstandingMessages: 10}},
		{name: "max outstanding bytes", opt: WithMaxOutstandingBytes(1024), want: gcppubsub.ReceiveSettings{MaxOutstandingBytes: 1024}},
		{name: "num goroutines", opt: WithNumGoroutines(4), want: gcppubsub.ReceiveSettings{NumGoroutines: 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcppubsub.ReceiveSettings{}
			tt.opt(&got)
			assert.Equal(t, tt.want, got)
		})
	}
}
