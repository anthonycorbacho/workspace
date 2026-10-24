package blob

// Errors returned by Blob implementations, so callers can handle them without knowing the storage used.
const (
	// ErrNotFound is returned when the file or the bucket does not exist.
	ErrNotFound = Error("not found")
	// ErrInternal is returned for any other storage failure.
	ErrInternal = Error("internal error")
)

// Error represents a blob error.
type Error string

// Error implements the error interface.
func (e Error) Error() string {
	return string(e)
}
