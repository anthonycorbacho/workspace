package blob

const (
	ErrNotFound = Error("not found")
	ErrInternal = Error("internal error")
)

// Error represents a blob error.
type Error string

func (e Error) Error() string {
	return string(e)
}
