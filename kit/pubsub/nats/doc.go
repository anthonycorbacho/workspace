// Package nats implements the pubsub publisher and subscriber with NATS JetStream.
//
// The streams and consumers must exist, they are not created. The publisher sets the subject
// header of the messages and the trace context headers (traceparent), the subscriptions are
// bound to a consumer with the explicit ack policy and join a queue group.
//
// Example:
//
//	nc, err := nats.Connect(url)
//	if err != nil {
//		return err
//	}
//	js, err := nc.JetStream()
//	if err != nil {
//		return err
//	}
//	consumer, err := js.ConsumerInfo("ORDERS", "orders-consumer")
//	if err != nil {
//		return err
//	}
//	subscriber, err := kitnats.NewSubscriber("orders-group", nc, js, consumer)
//	if err != nil {
//		return err
//	}
//	defer subscriber.Close()
//	err = subscriber.Subscribe(ctx, "orders.created", handler)
package nats
