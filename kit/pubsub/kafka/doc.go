// Package kafka implements the pubsub publisher and subscriber with Kafka (franz-go).
//
// The publisher sends the attributes and the trace context (traceparent) as record headers and
// the key (pubsub.WithKey) as record key. The subscriber polls the records in the background,
// commits the offsets of the acked records and consumes a partition again from a nacked record,
// after the retry backoff. With a dead letter topic, the records failing too many deliveries are
// published to it so they don't block their partition.
//
// Example:
//
//	client, err := kafka.NewClient([]string{"localhost:9092"},
//		kafka.WithConsumerGroup("orders"),
//		kafka.WithDeadLetter("{topic}.dlq", 5),
//	)
//	if err != nil {
//		return err
//	}
//	subscriber, err := kafka.NewSubscriber(client)
//	if err != nil {
//		return err
//	}
//	defer subscriber.Close()
//	err = subscriber.Subscribe(ctx, "orders", handler)
package kafka
