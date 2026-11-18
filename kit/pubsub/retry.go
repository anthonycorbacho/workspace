package pubsub

import (
	"errors"
	"math"
	"time"
)

// MetadataDeliveryAttempt is the metadata key of the delivery attempt of a message, 1 for the first
// delivery. It is set by the backends counting the deliveries: Kafka, NATS JetStream, and Google Cloud
// Pub/Sub when the subscription has a dead letter policy.
const MetadataDeliveryAttempt = "delivery-attempt"

// permanentError is an error that retrying the message will not fix.
type permanentError struct {
	err error
}

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as permanent: processing the message again will not succeed (eg: a malformed
// payload), so a nacked message is not redelivered. Depending on the backend, the message is:
//   - Kafka: published to the dead letter topic (see kafka.WithDeadLetter), or dropped and logged without one.
//   - NATS JetStream: terminated, it is not redelivered.
//   - Google Cloud Pub/Sub: nacked, the subscription dead letter policy stops its redelivery.
//
// Permanent returns nil if err is nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err, or an error of its chain, is marked as permanent (see Permanent).
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}

// Backoff is the delay before redelivering a nacked message, it doubles at each failed delivery from
// Initial up to Max. A zero Initial redelivers the messages right away.
type Backoff struct {
	Initial time.Duration `yaml:"initial"`
	Max     time.Duration `yaml:"max"`
}

// DefaultBackoff is the backoff of the backends redelivering the nacked messages (Kafka, NATS JetStream).
var DefaultBackoff = Backoff{Initial: 500 * time.Millisecond, Max: 30 * time.Second}

// Delay returns the delay before redelivering a message after its failed delivery attempt (from 1).
func (b Backoff) Delay(attempt int) time.Duration {
	if b.Initial <= 0 || attempt < 1 {
		return 0
	}
	d := b.Initial
	for i := 1; i < attempt; i++ {
		if (b.Max > 0 && d >= b.Max) || d > math.MaxInt64/2 {
			break
		}
		d *= 2
	}
	if b.Max > 0 && d > b.Max {
		d = b.Max
	}
	return d
}
