// Package errors provides simple error handling primitives.
//
// The traditional error handling idiom in Go is roughly akin to
//
//	if err != nil {
//	        return err
//	}
//
// which when applied recursively up the call stack results in error reports
// without context or debugging information. The errors package allows
// programmers to add context to the failure path in their code in a way
// that does not destroy the original value of the error.
//
// # Adding context to an error
//
// The errors.Wrap function returns a new error that prefixes the original error
// with the supplied message, the original error stays in the chain (errors.Is, errors.As).
// For example
//
//	_, err := io.ReadAll(r)
//	if err != nil {
//	        return errors.Wrap(err, "read failed")
//	}
//
// # Coded errors
//
// A coded error (see Error) is an error with a unique code, a Kind and a client facing message,
// defined in errors.defs.yaml. Its constructor is generated in error.gen.go and its documentation in docs/
// by running `make errgen` (go generate). The transports map the Kind to their own status
// (eg: kit/grpc to a gRPC code).
package errors

//go:generate go run ../../sdlc/errgen -input errors.defs.yaml -output error.gen.go -docs docs
