package errors

import (
	"fmt"

	"github.com/anthonycorbacho/workspace/api/errdetails"
	"github.com/golang/protobuf/proto" //nolint - required by st.WithDetails
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Status represents an RPC status code, message, and details. It is immutable.
// Code represent a set of Status code tht will be translated from gRPC to HTTP if the error is meant to be consumed by an HTTP client.
//
// | HTTP | gRPC                | Description                                                                                                                                                          |
// |------|---------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
// | 200  | OK                  | No error.                                                                                                                                                            |
// | 400  | INVALID_ARGUMENT    | Client specified an invalid argument. Check error message and error details for more information.                                                                    |
// | 400  | FAILED_PRECONDITION | Request can not be executed in the current system state, such as deleting a non-empty directory.                                                                     |
// | 400  | OUT_OF_RANGE        | Client specified an invalid range.                                                                                                                                   |
// | 401  | UNAUTHENTICATED     | Request not authenticated due to missing, invalid, or expired OAuth token.                                                                                           |
// | 403  | PERMISSION_DENIED   | Client does not have sufficient permission. This can happen because the OAuth token does not have the right scopes, the client doesn't have permission.              |
// | 404  | NOT_FOUND           | A specified resource is not found.                                                                                                                                   |
// | 409  | ABORTED             | Concurrency conflict, such as read-modify-write conflict.                                                                                                            |
// | 409  | ALREADY_EXISTS      | The resource that a client tried to create already exists.                                                                                                           |
// | 429  | RESOURCE_EXHAUSTED  | Either out of resource quota or reaching rate limiting.                                                                                                              |
// | 499  | CANCELLED           | Request cancelled by the client.                                                                                                                                     |
// | 500  | DATA_LOSS           | Unrecoverable data loss or data corruption. The client should report the error to the user.                                                                          |
// | 500  | UNKNOWN             | Unknown server error. Typically a server bug.                                                                                                                        |
// | 500  | INTERNAL            | Internal server error. Typically a server bug.                                                                                                                       |
// | 501  | NOT_IMPLEMENTED     | API method not implemented by the server.                                                                                                                            |
// | 502  | N/A                 | Network error occurred before reaching the server. Typically a network outage or misconfiguration.                                                                   |
// | 503  | UNAVAILABLE         | Service unavailable. Typically the server is down.                                                                                                                   |
// | 504  | DEADLINE_EXCEEDED   | Request deadline exceeded. This will happen only if the caller sets a deadline that is shorter than the method's default deadline.                                   |
//
// details provide details error information appended to the status. It is optional but always good to add detail when applicable.
func Status(code codes.Code, message string, details ...*errdetails.ErrorInfo) error {
	st := status.New(code, message)

	dd := make([]proto.Message, 0, len(details))
	for _, d := range details {
		dd = append(dd, d)
	}

	st, err := st.WithDetails(dd...)
	if err != nil {
		// If this errored, it will always error here, better panic,
		// so we can figure out why than have this silently passing.
		panic(fmt.Sprintf("unexpected error attaching metadata: %v", err))
	}

	return st.Err()
}
