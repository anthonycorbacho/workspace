package pubsub

// Pubsub errors.
const (
	// ErrPublisherClosed is returned when publishing with a closed publisher.
	ErrPublisherClosed = Error("publisher is closed")
	// ErrSubscriberClosed is returned when subscribing with a closed subscriber.
	ErrSubscriberClosed = Error("subscriber is closed")
	// ErrNilTopic is returned when publishing or subscribing to an empty topic.
	ErrNilTopic = Error("topic is nil")
	// ErrNilClient is returned when creating a publisher or a subscriber with a nil client.
	ErrNilClient = Error("pubsub client is nil")
)

// Error represents a pubsub error.
type Error string

// Error returns the error message.
func (e Error) Error() string {
	return string(e)
}
