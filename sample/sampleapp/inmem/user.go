package inmem

import (
	"context"
	"sync"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

// Verify interface compliance
var _ sampleapp.UserStorage = (*UserStorage)(nil)

// UserStorage a dummy way to store users.
//
// As a storage adapter, it returns coded errors (see kit/errors): this is where the storage failures
// (eg: a missing row, a unique constraint violation) are known and translated.
type UserStorage struct {
	// For the demo purpose the user data is the same as the domain user
	// but in real word application, we might want to have our own inmem user representation.
	// especially with DB since we will have custom attributes like createdAt etc.
	db map[string]sampleapp.User
	mu sync.RWMutex
}

func New() *UserStorage {
	return &UserStorage{
		db: map[string]sampleapp.User{
			"424242": {ID: "424242", Name: "Jean val Jean"},
		},
	}
}

func (u *UserStorage) Fetch(ctx context.Context, id string) (*sampleapp.User, error) {
	_, span := otel.Tracer("").Start(ctx, "inmem.fetch")
	defer span.End()
	u.mu.RLock()
	defer u.mu.RUnlock()

	usr, ok := u.db[id]
	if !ok {
		err := errors.SampleUserNotFound(id)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Message)
		return nil, err
	}

	// a copy is returned so the caller can't modify the stored user without the lock.
	return &usr, nil
}

func (u *UserStorage) Create(ctx context.Context, usr *sampleapp.User) error {
	_, span := otel.Tracer("").Start(ctx, "inmem.create")
	defer span.End()

	u.mu.Lock()
	defer u.mu.Unlock()

	// the user name is unique, as a unique constraint in a database.
	for _, existing := range u.db {
		if existing.Name == usr.Name {
			err := errors.SampleUserAlreadyExists(usr.Name)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Message)
			return err
		}
	}

	u.db[usr.ID] = *usr
	return nil
}

func (u *UserStorage) Delete(ctx context.Context, id string) error {
	_, span := otel.Tracer("").Start(ctx, "inmem.delete")
	defer span.End()

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, ok := u.db[id]; !ok {
		err := errors.SampleUserNotFound(id)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Message)
		return err
	}
	delete(u.db, id)
	return nil
}
