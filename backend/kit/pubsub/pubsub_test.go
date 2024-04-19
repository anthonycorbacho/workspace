package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCtx(t *testing.T) {
	ctx := WithTopic(context.Background(), "a.topic")
	topic := GetTopic(ctx)

	assert.Equal(t, "a.topic", topic)
}
