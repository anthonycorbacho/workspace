// Package gcp implements the pubsub publisher and subscriber with Google Cloud Pub/Sub.
//
// The topics and subscriptions must exist, they are not created. The publisher sets the topic
// attribute of the messages (see pubsub.Topic) and the trace context attributes (traceparent).
// The client connects to the emulator when PUBSUB_EMULATOR_HOST is set.
//
// Example:
//
//	client, err := gcppubsub.NewClient(ctx, "my-project")
//	if err != nil {
//		return err
//	}
//	publisher, err := gcp.NewPublisher(client)
//	if err != nil {
//		return err
//	}
//	defer publisher.Close()
//	err = publisher.Publish(ctx, "orders", pubsub.Message("hello"), pubsub.WithKey("customer-1"))
package gcp
