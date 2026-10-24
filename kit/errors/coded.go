package errors

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Error is a coded error: an error identified by a unique code (eg: ERR_01001),
// a kind (eg: KindNotFound) and a client facing message.
//
// Coded errors are defined in errors.defs.yaml and their constructors are generated
// in error.gen.go (see `make errgen`), eg:
//
//	return nil, errors.SampleUserNotFound(id).WithCause(err)
//
// The package is transport free: the transports map the kind to their own status (kit/grpc to a gRPC code,
// the gateway to an HTTP status). The code, the message and the details are sent to the client, the internal
// details and the cause are only part of the error string (eg: logs) and of the error chain (errors.Is, errors.As).
//
// Errors are immutable, WithDetail and WithCause return a copy.
type Error struct {
	// Code is the unique error code.
	Code string
	// Kind is the category of the error, mapped to a status by the transports.
	Kind Kind
	// Message is the client facing message.
	Message string
	// Details are the client facing structured details.
	Details map[string]any

	// internal details are never sent to the client.
	internal map[string]any
	// cause is the underlying error, never sent to the client.
	cause error
}

// Error returns the error string: code, message, internal details and cause.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Code)
	b.WriteString(": ")
	b.WriteString(e.Message)
	if len(e.internal) > 0 {
		b.WriteString(" (")
		for i, k := range slices.Sorted(maps.Keys(e.internal)) {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%v", k, e.internal[k])
		}
		b.WriteString(")")
	}
	if e.cause != nil {
		b.WriteString(": ")
		b.WriteString(e.cause.Error())
	}
	return b.String()
}

// Unwrap returns the cause of the error.
func (e *Error) Unwrap() error {
	return e.cause
}

// Is reports whether the target is a coded error with the same code,
// eg: errors.Is(err, errors.SampleUserNotFound("")).
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t != nil && e.Code == t.Code
}

// WithDetail returns a copy of the error with the given client facing detail.
func (e *Error) WithDetail(key string, value any) *Error {
	c := e.clone()
	c.Details[key] = value
	return c
}

// WithCause returns a copy of the error with the given cause.
// The cause is part of the error chain and of the error string, it is never sent to the client.
func (e *Error) WithCause(err error) *Error {
	c := e.clone()
	c.cause = err
	return c
}

func (e *Error) clone() *Error {
	c := *e
	c.Details = maps.Clone(e.Details)
	if c.Details == nil {
		c.Details = map[string]any{}
	}
	c.internal = maps.Clone(e.internal)
	return &c
}

// HasCode reports whether err or an error of its chain is the coded error with the given code,
// eg: errors.HasCode(err, errors.SampleUserNotFoundCode).
func HasCode(err error, code string) bool {
	return Is(err, &Error{Code: code})
}

// FromError returns the coded error of err:
//   - nil if err is nil.
//   - the coded error of the error chain if any.
//   - Unclassified with the KindCanceled or KindDeadlineExceeded kind for a context error.
//   - Internal for any other error, with err as cause so the client never sees its message.
//
// The errors received from another service are already coded errors (see kit/grpc).
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if As(err, &e) {
		return e
	}
	switch {
	case Is(err, context.Canceled):
		return NewUnclassified(KindCanceled, context.Canceled.Error()).WithCause(err)
	case Is(err, context.DeadlineExceeded):
		return NewUnclassified(KindDeadlineExceeded, context.DeadlineExceeded.Error()).WithCause(err)
	}
	return Internal().WithCause(err)
}

// NewUnclassified returns an Unclassified error with the given kind and message.
// It represents an error without an error of the catalog (eg: a context error, an error received from a
// third party service), prefer the errors of the catalog otherwise.
func NewUnclassified(kind Kind, message string) *Error {
	e := Unclassified()
	e.Kind = kind
	e.Message = message
	return e
}
