package cache

// Cache errors.
const (
	ErrKeyInvalid   = Error("cache key is not valid")
	ErrValueInvalid = Error("cache value is invalid")
	ErrNotFound     = Error("cache value not found")
)

// Error represents a cache error.
type Error string

// Error returns the error message.
func (e Error) Error() string {
	return string(e)
}
