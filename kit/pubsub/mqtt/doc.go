// Package mqtt implements the pubsub publisher and subscriber with MQTT v5 (paho).
//
// The publisher sends the attributes, the topic and the trace context (traceparent) as MQTT v5
// user properties, with the QoS 1 by default (see pubsub.WithQoS and pubsub.WithRetain).
// MQTT has no negative acknowledgement: a message the handler failed to process is not redelivered.
//
// The client reconnects automatically and restores successful Subscriber subscriptions, including
// wildcard filters, with their original QoS and retained-message settings. Failed restoration attempts
// are retried with WithRetry's delay until the client is disconnected. The original subscribe context
// only bounds registration; canceling it does not stop delivery or subscription restoration.
//
// Example:
//
//	client, err := mqtt.NewClient(ctx, "mqtt://localhost:1883", "my-service-1")
//	if err != nil {
//		return err
//	}
//	publisher, err := mqtt.NewPublisher(client)
//	if err != nil {
//		return err
//	}
//	defer publisher.Close()
//	err = publisher.Publish(ctx, "devices/1/state", pubsub.Message("on"), pubsub.WithQoS(2))
package mqtt
