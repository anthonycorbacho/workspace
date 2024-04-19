package main

import (
	"context"

	"github.com/anthonycorbacho/workspace/api/errdetails"
	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
	"google.golang.org/grpc/codes"
)

// grpcUser represent the grpc user service implementation.
type grpcUser struct {
	pb.UnsafeSampleAppServer
	service *sampleapp.UserService
	log     *log.Logger
}

func newGrpcUser(service *sampleapp.UserService) (*grpcUser, error) {

	if service == nil {
		return nil, errors.New("nil user service")
	}

	return &grpcUser{
		service: service,
		log:     log.L(),
	}, nil
}

func (u *grpcUser) Fetch(ctx context.Context, request *pb.FetchRequest) (*pb.FetchResponse, error) {

	user, err := u.service.Fetch(ctx, request.Id)
	if err != nil {
		u.log.Error(ctx, "fetching user", log.Error(err), log.String("user.id", request.Id))

		if errors.Is(err, sampleapp.ErrUserNotFound) {
			return nil, errors.Status(
				codes.NotFound,
				"user not found",
				&errdetails.ErrorInfo{
					Reason: "USER_NOT_FOUND",
					Metadata: map[string]string{
						"user.id": request.Id,
					},
				})
		}

		return nil, errors.Status(
			codes.Unknown,
			err.Error(), // log the full error to provide enough context
			&errdetails.ErrorInfo{
				Reason: "UNKNOWN_ERROR",
				Metadata: map[string]string{
					"user.id": request.Id,
				},
			})
	}
	return &pb.FetchResponse{
		Name: user.Name,
	}, nil
}

func (u *grpcUser) Create(ctx context.Context, request *pb.CreateRequest) (*pb.CreateResponse, error) {

	if err := request.Validate(); err != nil {
		u.log.Error(ctx, "creating user", log.Error(err))

		if errors.Is(err, sampleapp.ErrUserNameMissing) {
			return nil, errors.Status(
				codes.InvalidArgument,
				"name is required in order to create the user",
				&errdetails.ErrorInfo{
					Reason: "INVALID_REQUEST",
					Metadata: map[string]string{
						"request": request.String(),
					},
				})
		}

		if errors.Is(err, sampleapp.ErrUserAlreadyExist) {
			return nil, errors.Status(
				codes.AlreadyExists,
				"user already exists in the system",
				&errdetails.ErrorInfo{
					Reason: "INVALID_REQUEST",
					Metadata: map[string]string{
						"request": request.String(),
					},
				})
		}

		return nil, errors.Status(
			codes.InvalidArgument,
			err.Error(),
			&errdetails.ErrorInfo{
				Reason: "INVALID_REQUEST",
				Metadata: map[string]string{
					"request": request.String(),
				},
			})
	}

	user := sampleapp.User{
		Name: request.Name,
	}
	if err := u.service.Create(ctx, &user); err != nil {
		u.log.Info(ctx, "creating user", log.Error(err))
		return nil, errors.Status(
			codes.InvalidArgument,
			err.Error(),
			&errdetails.ErrorInfo{
				Reason: "FAIL_CREATE_USER",
				Metadata: map[string]string{
					"request": request.String(),
				},
			})
	}

	return &pb.CreateResponse{
		Id:   user.ID,
		Name: user.Name,
	}, nil
}

func (u *grpcUser) Delete(ctx context.Context, request *pb.DeleteRequest) (*pb.DeleteResponse, error) {

	if err := u.service.Delete(ctx, request.Id); err != nil {
		u.log.Error(ctx, "deleting user", log.Error(err), log.String("user.id", request.Id))

		if errors.Is(err, sampleapp.ErrUserNotFound) {
			return nil, errors.Status(
				codes.NotFound,
				"cannot delete unknown user",
				&errdetails.ErrorInfo{
					Reason: "USER_NOT_FOUND",
					Metadata: map[string]string{
						"user.id": request.Id,
					},
				})
		}

		return nil, errors.Status(
			codes.InvalidArgument,
			err.Error(),
			&errdetails.ErrorInfo{
				Reason: "FAIL_DELETE_USER",
				Metadata: map[string]string{
					"request": request.String(),
				},
			})
	}
	return &pb.DeleteResponse{}, nil
}
