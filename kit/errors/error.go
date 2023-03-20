package errors

import (
	stdErrors "errors"
	"fmt"
)

// New returns an error that formats as the given text.
// Each call to New returns a distinct error value even if the text is identical.
func New(message string) error {
	return stdErrors.New(message)
}

// Newf returns an error that formats according to a format specifier.
// Each call to Newf returns a distinct error value even if the text is identical.
func Newf(format string, args ...interface{}) error {
	return New(fmt.Sprintf(format, args...))
}

// Wrap returns an error annotating err with a stack trace
// at the point Wrap is called, and the supplied message.
// If err is nil, Wrap returns nil.
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", message, err)
}

// Wrapf returns an error annotating err with a stack trace
// at the point Wrapf is called, and the format specifier.
// If err is nil, Wrapf returns nil.
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	msg := fmt.Sprintf(format, args...)
	return Wrap(err, msg)
}

// Unwrap returns the result of calling the Unwrap method on err, if err's
// type contains an Unwrap method returning error.
// Otherwise, Unwrap returns nil.
func Unwrap(err error) error {
	u, ok := err.(interface{ Unwrap() error })
	if !ok {
		return nil
	}
	return u.Unwrap()
}

// Is reports whether any error in err's chain matches target.
//
// The chain consists of err itself followed by the sequence of errors obtained by
// repeatedly calling Unwrap.
//
// An error is considered to match a target if it is equal to that target or if
// it implements a method Is(error) bool such that Is(target) returns true.
func Is(err, target error) bool {
	// standard library provide a Is.
	// To avoid importing multiple  errors package, we proxy the
	// call to the standard errors so we keep the errors to the same package.
	return stdErrors.Is(err, target)
}

// As finds the first error in err's chain that matches target, and if one is found, sets
// target to that error value and returns true. Otherwise, it returns false.
//
// The chain consists of err itself followed by the sequence of errors obtained by
// repeatedly calling Unwrap.
//
// An error matches target if the error's concrete value is assignable to the value
// pointed to by target, or if the error has a method As(interface{}) bool such that
// As(target) returns true. In the latter case, the As method is responsible for
// setting target.
func As(err error, target interface{}) bool {
	// standard library provide a As.
	// To avoid importing multiple  errors package, we proxy the
	// call to the standard errors so we keep the errors to the same package.
	return stdErrors.As(err, target)
}
