// Package kafka implements the pubsub publisher and subscriber with Kafka (franz-go).
//
// The publisher sends the attributes and the trace context (traceparent) as record headers and
// the key (pubsub.WithKey) as record key. The subscriber polls the records in the background,
// commits the offsets of the acked records and consumes a partition again from a nacked record.
//
// Example:
//
//	client, err := kafka.NewClient([]string{"localhost:9092"}, kafka.WithConsumerGroup("orders"))
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
