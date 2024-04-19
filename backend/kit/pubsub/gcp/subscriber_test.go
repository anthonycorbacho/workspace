package gcp

import (
	"testing"
	"time"

	gcppubsub "cloud.google.com/go/pubsub"
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
		MaxExtension:           60 * time.Minute,
		MaxExtensionPeriod:     time.Minute,
		MinExtensionPeriod:     0,
		MaxOutstandingMessages: 42,
		MaxOutstandingBytes:    1e9,
		NumGoroutines:          1,
	}, s.settings)
}
