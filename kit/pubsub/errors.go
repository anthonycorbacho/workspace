package pubsub

// Pubsub errors.
const (
	ErrPublisherClosed  = Error("publisher is closed")
	ErrSubscriberClosed = Error("subscriber is closed")
	ErrNilTopic         = Error("topic is nil")
	ErrNilClient        = Error("pubsub client is nil")
)

// Deprecated error names, kept for backward compatibility.
const (
	// Deprecated: use ErrPublisherClosed.
	PublisherClosed = ErrPublisherClosed
	// Deprecated: use ErrSubscriberClosed.
	SubscriberCLosed = ErrSubscriberClosed
)

// Error represents a pubsub error.
type Error string

// Error returns the error message.
func (e Error) Error() string {
	return string(e)
}
