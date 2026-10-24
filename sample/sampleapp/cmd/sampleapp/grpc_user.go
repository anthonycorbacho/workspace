package main

import (
	"context"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
)

// grpcUser represent the grpc user service implementation.
//
// The requests are validated by the kit grpc server (protovalidate rules of the proto).
// The errors are coded errors created by the service and its storage (see kit/errors), they are
// returned as is: the kit grpc server sends them to the client, and turns any other error into an
// Internal error that is logged but never sent.
type grpcUser struct {
	pb.UnsafeSampleAppServiceServer
	service *sampleapp.UserService
}

func newGrpcUser(service *sampleapp.UserService) (*grpcUser, error) {
	if service == nil {
		return nil, errors.New("nil user service")
	}

	return &grpcUser{
		service: service,
	}, nil
}

func (u *grpcUser) Fetch(ctx context.Context, request *pb.FetchRequest) (*pb.FetchResponse, error) {
	user, err := u.service.Fetch(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	return &pb.FetchResponse{
		Name: user.Name,
	}, nil
}

func (u *grpcUser) Create(ctx context.Context, request *pb.CreateRequest) (*pb.CreateResponse, error) {
	user := sampleapp.User{
		Name: request.Name,
	}
	if err := u.service.Create(ctx, &user); err != nil {
		return nil, err
	}

	return &pb.CreateResponse{
		Id:   user.ID,
		Name: user.Name,
	}, nil
}

func (u *grpcUser) Delete(ctx context.Context, request *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	if err := u.service.Delete(ctx, request.Id); err != nil {
		return nil, err
	}
	return &pb.DeleteResponse{}, nil
}
