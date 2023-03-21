package inmem

import (
	"context"
	"fmt"
	"sync"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

// Verify interface compliance
var _ sampleapp.UserStorage = (*UserStorage)(nil)

// UserStorage a dummy way to store users.
type UserStorage struct {
	// For the demo purpose the user data is the same as the domain user
	// but in real word application, we might want to have our own inmem user representation.
	// especially with DB since we will have custom attributes like createdAt etc.
	db map[string]*sampleapp.User
	mu sync.RWMutex
}

func New() *UserStorage {
	db := make(map[string]*sampleapp.User)
	db["424242"] = &sampleapp.User{
		ID:   "424242",
		Name: "Jean val Jean",
	}
	return &UserStorage{
		db: db,
	}
}

func (u *UserStorage) Fetch(ctx context.Context, id string) (*sampleapp.User, error) {
	_, span := otel.Tracer("").Start(ctx, "inmem.fetch")
	defer span.End()
	u.mu.RLock()
	defer u.mu.RUnlock()

	usr, ok := u.db[id]
	if !ok {
		err := sampleapp.ErrUserNotFound
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("user %s doest exist", id))
		return nil, err
	}

	return usr, nil
}

func (u *UserStorage) Create(ctx context.Context, usr *sampleapp.User) error {
	_, span := otel.Tracer("").Start(ctx, "inmem.create")
	defer span.End()

	if err := usr.Validate(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return errors.Wrap(err, "inmem.create")
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	_, ok := u.db[usr.ID]
	if ok {
		return sampleapp.ErrUserAlreadyExist
	}

	u.db[usr.ID] = usr
	return nil
}

func (u *UserStorage) Delete(ctx context.Context, id string) error {
	_, span := otel.Tracer("").Start(ctx, "inmem.delete")
	defer span.End()
	_, err := u.Fetch(ctx, id)
	if err != nil {
		err = errors.Wrap(err, "inmem.create")
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("user %s not deleted", id))
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.db, id)

	return nil
}
