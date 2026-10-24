// Package config creates the pubsub publishers and subscribers from a configuration, loaded
// with kit/config (YAML and environment variables).
//
// Example:
//
//	kind: "kafka-subscriber"
//	kafkaSubscriber:
//	  brokers: ["localhost:9092"]
//	  consumerGroup: "orders"
//
//	var conf config.Config
//	if err := kitconfig.From(file, &conf); err != nil {
//		return err
//	}
//	subscriber, closeSubscriber, err := conf.Subscriber(ctx)
//	if err != nil {
//		return err
//	}
//	defer closeSubscriber()
package config
