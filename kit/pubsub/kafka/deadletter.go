package kafka

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/twmb/franz-go/pkg/kgo"
)

// The headers describing the failure of a dead letter record.
const (
	deadLetterTopicHeader     = "dead-letter-topic"
	deadLetterPartitionHeader = "dead-letter-partition"
	deadLetterOffsetHeader    = "dead-letter-offset"
	deadLetterAttemptsHeader  = "dead-letter-attempts"
	deadLetterErrorHeader     = "dead-letter-error"
)

// deadLetter is the dead letter topic of the records failing too many deliveries (see WithDeadLetter).
type deadLetter struct {
	topic       string
	maxAttempts int
}

// enabled reports whether the failing records are published to a dead letter topic.
func (d deadLetter) enabled() bool {
	return d.topic != ""
}

func (d deadLetter) validate() error {
	if !d.enabled() && d.maxAttempts == 0 {
		return nil
	}
	if d.topic == "" {
		return errors.New("dead letter topic is required")
	}
	if d.maxAttempts < 1 {
		return fmt.Errorf("dead letter max attempts must be at least 1, got %d", d.maxAttempts)
	}
	return nil
}

// topicOf returns the dead letter topic of the records of topic.
func (d deadLetter) topicOf(topic string) string {
	return strings.ReplaceAll(d.topic, "{topic}", topic)
}

// record returns the dead letter record of rec, which failed attempt deliveries with cause.
func (d deadLetter) record(rec *kgo.Record, attempt int, cause error) *kgo.Record {
	dl := &kgo.Record{
		Topic:   d.topicOf(rec.Topic),
		Key:     rec.Key,
		Value:   rec.Value,
		Headers: make([]kgo.RecordHeader, 0, len(rec.Headers)+5),
	}
	carrier := recordCarrier{dl}
	for _, h := range rec.Headers {
		carrier.Set(h.Key, string(h.Value))
	}
	carrier.Set(deadLetterTopicHeader, rec.Topic)
	carrier.Set(deadLetterPartitionHeader, strconv.Itoa(int(rec.Partition)))
	carrier.Set(deadLetterOffsetHeader, strconv.FormatInt(rec.Offset, 10))
	carrier.Set(deadLetterAttemptsHeader, strconv.Itoa(attempt))
	if cause != nil {
		carrier.Set(deadLetterErrorHeader, cause.Error())
	}
	return dl
}

// publishDeadLetter publishes the dead letter record of rec, it is traced as a child of the span of ctx
// (the processing of rec).
func (s *Subscriber) publishDeadLetter(ctx context.Context, rec *kgo.Record, attempt int, cause error) error {
	dl := s.client.deadLetter.record(rec, attempt, cause)
	ctx, span := startPublishSpan(ctx, tracer, dl)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := s.client.client.ProduceSync(ctx, dl).FirstErr(); err != nil {
		err = fmt.Errorf("publishing to the dead letter topic %s: %w", dl.Topic, err)
		tracing.RecordError(span, err)
		return err
	}
	return nil
}
