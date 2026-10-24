// Package pubsub contains the interfaces to publish messages to topics and to subscribe to them,
// implemented by the backends:
//
//   - kit/pubsub/gcp: Google Cloud Pub/Sub.
//   - kit/pubsub/nats: NATS JetStream.
//   - kit/pubsub/kafka: Kafka (and the Kafka compatible brokers like Redpanda).
//   - kit/pubsub/mqtt: MQTT v5.
//
// kit/pubsub/config creates the publishers and the subscribers from a configuration.
//
// # Acknowledgement
//
// Subscriber.Subscribe acks a message once the handler succeeded and nacks it when the handler
// returned an error, so the message is redelivered when the backend supports it (see
// AckOnSuccess): a message is never acked before it is processed. Subscriber.SubscribeWithAck
// leaves the acknowledgement to the handler. The backends differ:
//
//   - gcp: a nacked message is redelivered, possibly to another subscriber.
//   - nats: a nacked message is redelivered (Nak), the consumer must use the explicit ack policy.
//   - kafka: the offsets are committed in order: acking a record acks the previous records of its
//     partition, nacking a record consumes its partition again from the record.
//   - mqtt: there is no negative acknowledgement, ack and nack are no-ops and the message is
//     acknowledged to the broker once the handler returned.
//
// # Tracing
//
// The publishers start a producer span "publish <topic>" and inject its context in the message
// headers (attributes, headers, user properties) with the global OpenTelemetry propagator, set by
// kit/telemetry.NewTracer (W3C TraceContext, Baggage and B3): the messages carry the traceparent
// header. The subscribers extract it and handle each message with a consumer span
// "process <topic>", child of the producer span, so a trace continues across the services.
// The spans have the OpenTelemetry messaging attributes (messaging.system,
// messaging.destination.name...).
//
// Example:
//
//	err := publisher.Publish(ctx, "orders", pubsub.Message(payload), pubsub.WithAttributes(map[string]string{"version": "1"}))
//
//	err = subscriber.Subscribe(ctx, "orders-subscription", func(ctx context.Context, msg pubsub.Message, metadata map[string]string) error {
//		// ctx holds the process span and the topic (see Topic), metadata the message headers.
//		return handle(ctx, msg)
//	})
package pubsub
