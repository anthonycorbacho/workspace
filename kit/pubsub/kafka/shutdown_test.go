package kafka

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func awaitKafkaResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Kafka subscriber")
		var zero T
		return zero
	}
}

func TestSubscriberCloseJoinsHandler(t *testing.T) {
	for _, regex := range []bool{false, true} {
		name, topic, subscription := "topic", "orders", "orders"
		var opts []Option
		if regex {
			name, topic, subscription = "regex", "orders.created", "^orders\\..*"
			opts = append(opts, WithRegex([]string{subscription}))
		}
		t.Run(name, func(t *testing.T) {
			client, err := NewClient([]string{unreachableBroker}, opts...)
			require.NoError(t, err)
			fetches := make(chan kgo.Fetches, 1)
			s := newSubscriber(client, func(ctx context.Context) kgo.Fetches {
				select {
				case f := <-fetches:
					return f
				case <-ctx.Done():
					return kgo.NewErrFetch(ctx.Err())
				}
			})

			proceed := make(chan struct{})
			release := sync.OnceFunc(func() { close(proceed) })
			t.Cleanup(func() {
				release()
				closed := make(chan error, 1)
				go func() { closed <- s.Close() }()
				assert.NoError(t, awaitKafkaResult(t, closed))
				awaitKafkaResult(t, s.pollerDone)
			})

			entered := make(chan context.Context, 1)
			canceled := make(chan error, 1)
			finished := make(chan struct{})
			var calls atomic.Int32
			handler := func(ctx context.Context, msg pubsub.Message, _ map[string]string) error {
				calls.Add(1)
				if msg.String() != "first" {
					return nil
				}
				defer close(finished)
				entered <- ctx
				select {
				case <-ctx.Done():
					canceled <- ctx.Err()
				case <-proceed:
					return nil
				}
				// Model cleanup after cancellation: Close must wait for it to finish.
				<-proceed
				return ctx.Err()
			}
			require.NoError(t, s.Subscribe(context.Background(), subscription, handler))
			// Nacking the first record must not hide delivery of a buffered record in another partition.
			fetches <- kgo.Fetches{{Topics: []kgo.FetchTopic{{
				Topic: topic,
				Partitions: []kgo.FetchPartition{
					{Records: []*kgo.Record{{Topic: topic, Value: []byte("first")}}},
					{Partition: 1, Records: []*kgo.Record{{Topic: topic, Partition: 1, Value: []byte("buffered")}}},
				},
			}}}}
			handlerCtx := awaitKafkaResult(t, entered)
			require.NoError(t, handlerCtx.Err())
			assert.Equal(t, topic, pubsub.Topic(handlerCtx))

			const closers = 8
			closed := make(chan error, closers)
			for range closers {
				go func() { closed <- s.Close() }()
			}
			require.ErrorIs(t, awaitKafkaResult(t, canceled), context.Canceled)
			assert.ErrorIs(t, context.Cause(handlerCtx), context.Canceled)
			assert.NoError(t, client.client.Context().Err(), "the client stays open until the handler returns")
			select {
			case <-closed:
				t.Fatal("Close returned before the running handler finished")
			case <-time.After(50 * time.Millisecond):
			}

			release()
			for range closers {
				require.NoError(t, awaitKafkaResult(t, closed))
			}
			select {
			case <-finished:
			default:
				t.Fatal("Close did not join the handler")
			}
			assert.EqualValues(t, 1, calls.Load(), "buffered records are not delivered during shutdown")
			assert.ErrorIs(t, client.client.Context().Err(), context.Canceled)
			assert.NoError(t, s.Close(), "repeated Close waits for the same completed shutdown")
			assert.ErrorIs(t, s.Subscribe(context.Background(), subscription, handler), pubsub.ErrSubscriberClosed)
		})
	}
}

func TestSubscriberCloseInterruptsPoll(t *testing.T) {
	client, err := NewClient([]string{unreachableBroker})
	require.NoError(t, err)
	polling := make(chan struct{})
	signalPolling := sync.OnceFunc(func() { close(polling) })
	s := newSubscriber(client, func(ctx context.Context) kgo.Fetches {
		signalPolling()
		return client.client.PollFetches(ctx)
	})
	t.Cleanup(func() {
		closed := make(chan error, 1)
		go func() { closed <- s.Close() }()
		assert.NoError(t, awaitKafkaResult(t, closed))
		awaitKafkaResult(t, s.pollerDone)
	})
	closed := make(chan error, 1)
	awaitKafkaResult(t, polling)
	go func() { closed <- s.Close() }()
	require.NoError(t, awaitKafkaResult(t, closed))
	select {
	case <-s.pollerDone:
	default:
		t.Fatal("Close returned before the blocked poller exited")
	}
}
