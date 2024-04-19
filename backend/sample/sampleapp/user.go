package sampleapp

import (
	"context"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/id"
	"go.opentelemetry.io/otel"
)

// User represent a simpleapp User
type User struct {
	ID   string
	Name string
}

// Validate is a simple helper function that can be uses to validate a user struct data.
func (u *User) Validate() error {
	if len(u.Name) == 0 {
		return ErrUserNameMissing
	}
	return nil
}

// UserStorage defines how we store a user.
// By definition, it is decouple from any backend implementation and its design to be an
// abstraction of an actual storage.
type UserStorage interface {
	Fetch(ctx context.Context, id string) (*User, error)
	Create(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
}

// UserService represent the service that manage user
// without implementation detail, this aim to provide API that will be exposed to the handler (HTTP, GRPC)
//
// This part is really up to the implementation detail and expectation.
// For instance, we could have this service at the handler level if we plan to only use one api exposure.
type UserService struct {
	// storage is the representation about how we store a user
	// this does not expose the implementation detail.
	// the backend implementation could be in memory, DB or other.
	storage UserStorage

	// We can imagine having other fields here
	// that will provide interaction with the service
	// cache		UserStorage // imagine having a RLU cache or redis Cache.
	// event 		UserEvent
	// notification UserNotification
}

// New create a new User service.
func New(storage UserStorage) *UserService {
	return &UserService{
		storage: storage,
	}
}

// The implementation below is pretty simple
// The real world use case will involve more pieces.
// this is only designed for demo purpose.

// Fetch fetches a single user by the given ID.
func (u *UserService) Fetch(ctx context.Context, id string) (*User, error) {
	const op = "user.user"
	ctx, span := otel.Tracer("").Start(ctx, op)
	defer span.End()

	usr, err := u.storage.Fetch(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	return usr, nil
}

// Create creates a new user.
// If the user already exist in the storage, ErrUserAlreadyExist will be returned.
func (u *UserService) Create(ctx context.Context, usr *User) error {
	const op = "user.create"
	ctx, span := otel.Tracer("").Start(ctx, op)
	defer span.End()

	// Assign to the user an ID.
	usr.ID = id.New()

	err := u.storage.Create(ctx, usr)
	if err != nil {
		return errors.Wrap(err, op)
	}

	return nil
}

// Delete deletes the user form the user storage.
// If the user does not exist, ErrUserNotFound will be returned.
func (u *UserService) Delete(ctx context.Context, id string) error {
	const op = "user.delete"
	ctx, span := otel.Tracer("").Start(ctx, op)
	defer span.End()

	err := u.storage.Delete(ctx, id)
	if err != nil {
		return errors.Wrap(err, op)
	}

	return nil
}
