package errors

// Kind is the category of a coded error, it tells the caller how to react to the error
// (eg: fix the request, retry later) independently of the transport.
//
// The kinds follow the canonical error codes of gRPC (https://grpc.io/docs/guides/status-codes/),
// the transports map them to their own status (eg: kit/grpc to a gRPC code, the gateway to an HTTP status).
type Kind int

const (
	// KindUnknown is an error without more information.
	KindUnknown Kind = iota
	// KindInvalidArgument is a request with an invalid argument (eg: a malformed field).
	KindInvalidArgument
	// KindNotFound is a requested entity that is not found.
	KindNotFound
	// KindAlreadyExists is an entity that the request tried to create and that already exists.
	KindAlreadyExists
	// KindFailedPrecondition is a request rejected because the system is not in the required state
	// (eg: deleting a non-empty directory).
	KindFailedPrecondition
	// KindAborted is an operation aborted by a concurrency conflict (eg: a read-modify-write conflict),
	// the request can be retried.
	KindAborted
	// KindOutOfRange is an operation attempted past the valid range (eg: reading past the end of a file).
	KindOutOfRange
	// KindUnauthenticated is a request without valid authentication credentials.
	KindUnauthenticated
	// KindPermissionDenied is a request not allowed for the authenticated caller.
	KindPermissionDenied
	// KindResourceExhausted is a resource exhausted (eg: a quota, a rate limit).
	KindResourceExhausted
	// KindCanceled is an operation canceled by the caller.
	KindCanceled
	// KindDeadlineExceeded is an operation that did not complete before its deadline.
	KindDeadlineExceeded
	// KindUnimplemented is an operation not implemented or not supported.
	KindUnimplemented
	// KindUnavailable is a service temporarily unavailable, the request can be retried.
	KindUnavailable
	// KindInternal is an internal error, an invariant of the system is broken.
	KindInternal
	// KindDataLoss is an unrecoverable data loss or corruption.
	KindDataLoss
)

var _kindNames = map[Kind]string{
	KindUnknown:            "UNKNOWN",
	KindInvalidArgument:    "INVALID_ARGUMENT",
	KindNotFound:           "NOT_FOUND",
	KindAlreadyExists:      "ALREADY_EXISTS",
	KindFailedPrecondition: "FAILED_PRECONDITION",
	KindAborted:            "ABORTED",
	KindOutOfRange:         "OUT_OF_RANGE",
	KindUnauthenticated:    "UNAUTHENTICATED",
	KindPermissionDenied:   "PERMISSION_DENIED",
	KindResourceExhausted:  "RESOURCE_EXHAUSTED",
	KindCanceled:           "CANCELLED",
	KindDeadlineExceeded:   "DEADLINE_EXCEEDED",
	KindUnimplemented:      "UNIMPLEMENTED",
	KindUnavailable:        "UNAVAILABLE",
	KindInternal:           "INTERNAL",
	KindDataLoss:           "DATA_LOSS",
}

// String returns the name of the kind as written in the error catalog (eg: NOT_FOUND).
func (k Kind) String() string {
	if name, ok := _kindNames[k]; ok {
		return name
	}
	return "UNKNOWN"
}
