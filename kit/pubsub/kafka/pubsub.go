package kafka

import (
	"context"
	"strconv"

	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// tracer is the tracer of the Kafka publishers and subscribers.
var tracer = otel.Tracer("kit/pubsub/kafka")

// recordCarrier adapts the headers of a Kafka record to propagation.TextMapCarrier.
type recordCarrier struct {
	record *kgo.Record
}

var _ propagation.TextMapCarrier = recordCarrier{}

// Get returns the value of the first header with key.
func (c recordCarrier) Get(key string) string {
	for _, h := range c.record.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// Set sets the value of the header with key, replacing the headers with the same key.
func (c recordCarrier) Set(key, value string) {
	headers := c.record.Headers[:0]
	for _, h := range c.record.Headers {
		if h.Key != key {
			headers = append(headers, h)
		}
	}
	c.record.Headers = append(headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

// Keys returns the keys of the headers.
func (c recordCarrier) Keys() []string {
	keys := make([]string, 0, len(c.record.Headers))
	for _, h := range c.record.Headers {
		keys = append(keys, h.Key)
	}
	return keys
}

// startPublishSpan starts the span publishing record, the span context is injected in the record headers.
func startPublishSpan(ctx context.Context, tracer trace.Tracer, record *kgo.Record) (context.Context, trace.Span) {
	return tracing.StartPublish(ctx, tracer, semconv.MessagingSystemKafka, record.Topic, recordCarrier{record}, recordAttributes(record)...)
}

// startProcessSpan starts the span processing record, child of the span the record was published with.
func startProcessSpan(ctx context.Context, tracer trace.Tracer, record *kgo.Record) (context.Context, trace.Span) {
	attrs := append(recordAttributes(record),
		semconv.MessagingDestinationPartitionID(strconv.Itoa(int(record.Partition))),
		semconv.MessagingKafkaOffset(int(record.Offset)),
	)
	return tracing.StartProcess(ctx, tracer, semconv.MessagingSystemKafka, record.Topic, recordCarrier{record}, attrs...)
}

// recordAttributes returns the span attributes of record.
func recordAttributes(record *kgo.Record) []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.MessagingMessageBodySize(len(record.Value))}
	if len(record.Key) > 0 {
		attrs = append(attrs, semconv.MessagingKafkaMessageKey(string(record.Key)))
	}
	return attrs
}
