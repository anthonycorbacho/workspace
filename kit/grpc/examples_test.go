package grpc_test

import (
	"context"
	"fmt"
	"time"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	grpckit "github.com/anthonycorbacho/workspace/kit/grpc"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
)

// Sample client using the default configuration.
func ExampleNewClient() {
	cc, err := grpckit.NewClient(
		"service.namespace.svc.cluster.local:8081",               // address of the grpc service
		grpc.WithTransportCredentials(insecure.NewCredentials()), // required transport cred, set to insecure
	)
	if err != nil {
		// handle error
	}

	// Pass the grpc client connection to the grpc client API
	client := pb.NewSampleAppClient(cc)

	// perform a client request
	resp, err := client.Fetch(
		context.Background(),
		&pb.FetchRequest{Id: "43"},
	)
	if err != nil {
		// handle client call error from the grpc server
	}

	// handle the client response
	fmt.Println(resp)
}

// Sample client using the default configuration.
// and pass retry upon failure.
// By default, retry on `ResourceExhausted` and `Unavailable` gRPC codes, use a 50ms
// linear backoff with 10% jitter.
func ExampleNewClient_withRetry() {
	cc, err := grpckit.NewClient(
		"service.namespace.svc.cluster.local:8081",               // address of the grpc service
		grpc.WithTransportCredentials(insecure.NewCredentials()), // required transport cred, set to insecure
	)
	if err != nil {
		// handle error
	}

	// Pass the grpc client connection to the grpc client API
	client := pb.NewSampleAppClient(cc)

	// perform a client request
	resp, err := client.Fetch(
		context.Background(),
		&pb.FetchRequest{Id: "43"},
		grpckit.WithMaxRetries(5), // retry 5 times
	)
	if err != nil {
		// handle client call error from the grpc server
	}

	// handle the client response
	fmt.Println(resp)
}

// Sample client using the default configuration.
// and pass retry upon failure.
// Retry on `ResourceExhausted` and `Unavailable` and `Aborted` gRPC codes, use a 50ms
// linear backoff with 10% jitter.
func ExampleNewClient_withRetryAndCodes() {
	cc, err := grpckit.NewClient(
		"service.namespace.svc.cluster.local:8081",               // address of the grpc service
		grpc.WithTransportCredentials(insecure.NewCredentials()), // required transport cred, set to insecure
	)
	if err != nil {
		// handle error
	}

	// Pass the grpc client connection to the grpc client API
	client := pb.NewSampleAppClient(cc)

	// perform a client request
	resp, err := client.Fetch(
		context.Background(),
		&pb.FetchRequest{Id: "43"},
		grpckit.WithMaxRetries(5),        // retry 5 times
		grpckit.WithCodes(codes.Aborted), // add a new code to the list of retry
	)
	if err != nil {
		// handle client call error from the grpc server
	}

	// handle the client response
	fmt.Println(resp)
}

// Client with custom retry that apply retry to all endpoints
func ExampleNewClient_withCustomRetry() {
	cc, err := grpckit.NewClient(
		"service.namespace.svc.cluster.local:8081",               // address of the grpc service
		grpc.WithTransportCredentials(insecure.NewCredentials()), // required transport cred, set to insecure
		grpc.WithChainUnaryInterceptor(
			// define a custom retry interceptor
			grpcretry.UnaryClientInterceptor(
				// define a new retry of backoff; Exponential backoff with jitter
				grpcretry.WithBackoff(grpcretry.BackoffExponentialWithJitter(50*time.Millisecond, 0.1)),
				grpckit.WithMaxRetries(5),        // retry policy for all endpoints
				grpckit.WithCodes(codes.Aborted), // retry policy code for all endpoints
			),
		),
	)
	if err != nil {
		// handle error
	}

	// Pass the grpc client connection to the grpc client API
	client := pb.NewSampleAppClient(cc)

	// perform a client request
	// if the request fail with status `ResourceExhausted` or `Unavailable` or `Aborted`,
	// it will retry 5 times exponentially.
	resp, err := client.Fetch(
		context.Background(),
		&pb.FetchRequest{Id: "43"},
	)
	if err != nil {
		// handle client call error from the grpc server
	}

	// handle the client response
	fmt.Println(resp)
}
