package pubsub

// Pubsub errors.
const (
	PublisherClosed  = Error("publisher is closed")
	SubscriberCLosed = Error("subscriber is closed")
)

// Error represents a cache error.
type Error string

// Error returns the error message.
func (e Error) Error() string {
	return string(e)
}
