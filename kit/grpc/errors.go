package grpc

import (
	"context"
	stderrors "errors"
	"io"

	"buf.build/go/protovalidate"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// unaryServerErrorInterceptor converts the errors returned by the handlers to coded errors (see errors.FromError).
func unaryServerErrorInterceptor(l *log.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			return resp, serverError(ctx, l, info.FullMethod, err)
		}
		return resp, nil
	}
}

// streamServerErrorInterceptor converts the errors returned by the handlers to coded errors (see errors.FromError).
func streamServerErrorInterceptor(l *log.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := handler(srv, ss); err != nil {
			return serverError(ss.Context(), l, info.FullMethod, err)
		}
		return nil
	}
}

// serverError returns the gRPC status error of the coded error sent to the client.
// Plain errors become an Internal error so their message (eg: a SQL query or a host name) is never
// sent to the client, server errors are logged with their cause and the request trace.
func serverError(ctx context.Context, l *log.Logger, method string, err error) error {
	coded := FromError(err)
	switch coded.Kind {
	case errors.KindInternal, errors.KindUnknown, errors.KindDataLoss:
		l.Error(ctx, "grpc request failed", log.String("grpc.method", method), log.Error(err))
	}
	return Status(coded).Err()
}

// recoverFrom logs the panic and returns an Internal error, the panic value is never sent to the client.
func recoverFrom(l *log.Logger) func(ctx context.Context, p any) error {
	return func(ctx context.Context, p any) error {
		l.Error(ctx, "grpc recover panic", log.Any("panic", p))
		return Status(errors.Internal()).Err()
	}
}

// validate validates a request with its protovalidate rules,
// the violations are returned in the details of an InvalidRequest error.
func validate(msg any) error {
	m, ok := msg.(proto.Message)
	if !ok {
		return nil
	}
	err := protovalidate.Validate(m)
	if err == nil {
		return nil
	}
	var validationErr *protovalidate.ValidationError
	if !stderrors.As(err, &validationErr) {
		// the rules can't be evaluated (eg: invalid CEL expression), it is not a client error.
		return errors.Internal().WithCause(err)
	}
	violations := make([]any, 0, len(validationErr.Violations))
	for _, v := range validationErr.Violations {
		violations = append(violations, map[string]any{
			"field":   protovalidate.FieldPathString(v.Proto.GetField()),
			"message": v.Proto.GetMessage(),
		})
	}
	return errors.InvalidRequest().WithDetail("violations", violations).WithCause(err)
}

// unaryServerValidatorInterceptor validates the incoming requests.
func unaryServerValidatorInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := validate(req); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// streamServerValidatorInterceptor validates the messages received on the stream.
func streamServerValidatorInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &validatingServerStream{ServerStream: ss})
	}
}

type validatingServerStream struct {
	grpc.ServerStream
}

func (s *validatingServerStream) RecvMsg(m any) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	return validate(m)
}

// unaryClientValidatorInterceptor validates the outgoing requests,
// so invalid requests are rejected before reaching the server.
func unaryClientValidatorInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if err := validate(req); err != nil {
			// sent as a status like a server error, so the metrics and the retries see its gRPC code.
			return Status(errors.FromError(err)).Err()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// unaryClientErrorInterceptor converts the errors received from the server to coded errors,
// so errors.Is and errors.As work across services and a coded error returned as is keeps its code.
func unaryClientErrorInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return clientError(invoker(ctx, method, req, reply, cc, opts...))
	}
}

// streamClientErrorInterceptor converts the errors received on the stream to coded errors.
func streamClientErrorInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, clientError(err)
		}
		return &codedClientStream{ClientStream: stream}, nil
	}
}

type codedClientStream struct {
	grpc.ClientStream
}

func (s *codedClientStream) RecvMsg(m any) error {
	return clientError(s.ClientStream.RecvMsg(m))
}

func (s *codedClientStream) SendMsg(m any) error {
	return clientError(s.ClientStream.SendMsg(m))
}

func (s *codedClientStream) CloseSend() error {
	return clientError(s.ClientStream.CloseSend())
}

// clientError returns the coded error of err, io.EOF is kept as it marks the end of a stream.
func clientError(err error) error {
	if err == nil || stderrors.Is(err, io.EOF) {
		return err
	}
	return FromError(err)
}
