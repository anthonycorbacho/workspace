package retry

const (
	// ErrPermanent when the error is permanent.
	ErrPermanent = Error("permanent")
)

// Error represents a retry error.
type Error string

// Error returns the error message.
func (e Error) Error() string {
	return string(e)
}
