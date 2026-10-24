package grpc

import (
	"fmt"

	errdetailsv1 "github.com/anthonycorbacho/workspace/api/errdetails/v1"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// _kindCodes maps the kinds of the coded errors to the gRPC codes, kit/errors is transport free so the
// mapping lives in the transport.
var _kindCodes = map[errors.Kind]codes.Code{
	errors.KindUnknown:            codes.Unknown,
	errors.KindInvalidArgument:    codes.InvalidArgument,
	errors.KindNotFound:           codes.NotFound,
	errors.KindAlreadyExists:      codes.AlreadyExists,
	errors.KindFailedPrecondition: codes.FailedPrecondition,
	errors.KindAborted:            codes.Aborted,
	errors.KindOutOfRange:         codes.OutOfRange,
	errors.KindUnauthenticated:    codes.Unauthenticated,
	errors.KindPermissionDenied:   codes.PermissionDenied,
	errors.KindResourceExhausted:  codes.ResourceExhausted,
	errors.KindCanceled:           codes.Canceled,
	errors.KindDeadlineExceeded:   codes.DeadlineExceeded,
	errors.KindUnimplemented:      codes.Unimplemented,
	errors.KindUnavailable:        codes.Unavailable,
	errors.KindInternal:           codes.Internal,
	errors.KindDataLoss:           codes.DataLoss,
}

// _codeKinds is the reverse mapping of _kindCodes.
var _codeKinds = func() map[codes.Code]errors.Kind {
	m := make(map[codes.Code]errors.Kind, len(_kindCodes))
	for k, c := range _kindCodes {
		m[c] = k
	}
	return m
}()

// Code returns the gRPC code of a kind.
func Code(k errors.Kind) codes.Code {
	if c, ok := _kindCodes[k]; ok {
		return c
	}
	return codes.Unknown
}

// Status returns the gRPC status of a coded error: the code of its kind, its message and an
// errdetails.v1.Error detail with its code, message and client facing details.
// The cause and the internal details are never sent.
func Status(e *errors.Error) *status.Status {
	st := status.New(Code(e.Kind), e.Message)
	withDetails, err := st.WithDetails(&errdetailsv1.Error{
		Code:    e.Code,
		Message: e.Message,
		Details: toStruct(e.Details),
	})
	if err != nil {
		// details can't be attached to an OK status, which is not an error.
		return st
	}
	return withDetails
}

// FromStatus returns the coded error of a gRPC status:
//   - the coded error carried by its errdetails.v1.Error detail (eg: returned by another service).
//   - Internal for an internal error without coded error, its message may leak internal information.
//   - Unclassified with the kind and message of the status otherwise (eg: a third party service).
func FromStatus(st *status.Status) *errors.Error {
	if st.Code() == codes.OK {
		return nil
	}
	kind, ok := _codeKinds[st.Code()]
	if !ok {
		kind = errors.KindUnknown
	}
	for _, d := range st.Details() {
		if ed, ok := d.(*errdetailsv1.Error); ok {
			return &errors.Error{
				Code:    ed.GetCode(),
				Kind:    kind,
				Message: ed.GetMessage(),
				Details: ed.GetDetails().AsMap(),
			}
		}
	}
	switch kind {
	case errors.KindUnknown, errors.KindInternal, errors.KindDataLoss:
		return errors.Internal()
	}
	return errors.NewUnclassified(kind, st.Message())
}

// FromError returns the coded error of err (see errors.FromError), a gRPC status error is converted with
// FromStatus, the original error is kept as cause.
func FromError(err error) *errors.Error {
	if err == nil {
		return nil
	}
	var e *errors.Error
	if errors.As(err, &e) {
		return e
	}
	if st, ok := status.FromError(err); ok {
		if coded := FromStatus(st); coded != nil {
			return coded.WithCause(err)
		}
	}
	return errors.FromError(err)
}

// toStruct converts the details to a protobuf struct,
// values not supported by protobuf (eg: a custom type) are converted to their string representation.
func toStruct(details map[string]any) *structpb.Struct {
	fields := make(map[string]*structpb.Value, len(details))
	for k, v := range details {
		value, err := structpb.NewValue(v)
		if err != nil {
			value = structpb.NewStringValue(fmt.Sprint(v))
		}
		fields[k] = value
	}
	return &structpb.Struct{Fields: fields}
}
