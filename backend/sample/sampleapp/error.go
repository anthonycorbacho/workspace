package sampleapp

const (
	// ErrUserNotFound when user is not found.
	ErrUserNotFound = Error("user not found")
	// ErrUserNameMissing when username is missing.
	ErrUserNameMissing = Error("user name is missing")
	// ErrUserAlreadyExist when user already exist in the system.
	ErrUserAlreadyExist = Error("user already exists")
)

// Error represents a Sampleapp error.
type Error string

// Error returns the error message.
func (e Error) Error() string {
	return string(e)
}
