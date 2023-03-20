package dlock

const (
	ErrAcquiredLock = Error("failed to acquired lock")
)

// Error represents a distributed lock error.
type Error string

func (e Error) Error() string {
	return string(e)
}
