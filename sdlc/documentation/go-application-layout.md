# Go Application Layout
By following a few simple rules we can decouple the application code, make it easier to test, and bring a consistent structure to the project.

The package strategy that we use for the projects involves 4 simple tenets:

1. Root package is for domain types  
2. Group subpackages by dependency
3. Use a shared _mock_ subpackage
4. _Main_ package ties together dependencies

These rules help isolate the packages and define a clear domain language across the entire application. An example/demo implementation can be found [here](https://github.com/anthonycorbacho/workspace/tree/main/sample/sampleapp).

* [Root package is for domain types](#root-package-is-for-domain-types)
* [Group subpackages by dependency](#group-subpackages-by-dependency)
* [Use a shared mock subpackage](#use-a-shared-mock-subpackage)
* [Main package ties together dependencies](#main-package-ties-together-dependencies)

## Root package is for domain types
The application has a logical, high-level language that describes how data and processes interact. This is the domain. If we have an e-commerce application the domain involves things like customers, accounts, charging credit cards, and handling inventory. If we are Facebook then the domain is users, likes, and relationships. It is the stuff that doesn't depend on the underlying technology.

We will place the domain types in the root package. This package only contains simple data types like a _User_ struct for holding user data or a _UserStorage/UserRepository_ interface for fetching or saving user data.

It may look something like (see [sample/sampleapp/user.go](../../sample/sampleapp/user.go)):

```go
package sampleapp

// User represents a sampleapp user.
type User struct {
	ID   string
	Name string
}

// UserStorage defines how we store a user, decoupled from any backend implementation.
type UserStorage interface {
	Fetch(ctx context.Context, id string) (*User, error)
	Create(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
}

// UserService manages the users, it is exposed by the handlers (HTTP, gRPC).
type UserService struct {
	storage UserStorage
}

// New creates a new user service.
func New(storage UserStorage) *UserService {
	return &UserService{storage: storage}
}
```

This makes the root package extremely simple. We may also include types that perform actions but only if they solely depend on other domain types. For example, you could have a type that polls the _UserService_ periodically. However, it should not call out to external services or save to a database. That is an implementation detail.

The root package should not depend on any other package of the application, nor on a transport: it only uses the
kit packages free of dependencies (eg: `kit/errors`, `kit/id`), the CI (depguard) rejects the gRPC imports.

## Group subpackages by dependency
If the root package is not allowed to have external dependencies then we must push those dependencies to subpackages. In this approach to package layout, subpackages exist as an adapter between our domain and the implementation.

For example, the _UserStorage_ might be backed by PostgreSQL. We can introduce a postgres subpackage in the application that provides a _postgres.UserStorage_ implementation, using [kit/sql](../../kit/sql):

```go
package postgres

import (
	"context"
	stdsql "database/sql"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/sql"
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
)

// Verify interface compliance.
var _ sampleapp.UserStorage = (*UserStorage)(nil)

// UserStorage represents a PostgreSQL implementation of sampleapp.UserStorage.
type UserStorage struct {
	db sql.Queryable
}

// New returns a UserStorage running its queries on db, a *sql.DB or a transaction.
func New(db sql.Queryable) *UserStorage {
	return &UserStorage{db: db}
}

// Fetch returns the user of the given id.
func (s *UserStorage) Fetch(ctx context.Context, id string) (*sampleapp.User, error) {
	var u sampleapp.User
	err := s.db.GetContext(ctx, &u, `SELECT id, name FROM users WHERE id = $1`, id)
	if errors.Is(err, stdsql.ErrNoRows) {
		return nil, errors.SampleUserNotFound(id).WithCause(err)
	}
	if err != nil {
		return nil, errors.Wrap(err, "selecting user")
	}
	return &u, nil
}

// implement remaining sampleapp.UserStorage interface...
```

The adapter translates the dependency errors to coded errors, see [application-error.md](application-error.md).

This isolates the PostgreSQL dependency which simplifies testing and provides an easy way to migrate to another database in the future. It can be used as a pluggable architecture if we decide to support other database implementations such as MySQL, or an in-memory one like the sample does ([sample/sampleapp/inmem](../../sample/sampleapp/inmem)).

It also gives you a way to layer implementations. Perhaps we want to hold an in-memory, [LRU cache](https://en.wikipedia.org/wiki/Cache_algorithms) in front of PostgreSQL. We can add a _UserCache_ that implements _UserStorage_ which can wrap the PostgreSQL implementation:

```go
package sampleapp

// UserCache wraps a UserStorage to provide an in-memory cache.
type UserCache struct {
	mu      sync.RWMutex
	cache   map[string]*User
	storage UserStorage
}

// NewUserCache returns a new read-through cache for storage.
func NewUserCache(storage UserStorage) *UserCache {
	return &UserCache{
		cache:   make(map[string]*User),
		storage: storage,
	}
}

// Fetch returns the user of the given id, the cached instance if available.
func (c *UserCache) Fetch(ctx context.Context, id string) (*User, error) {
	// Check the local cache first.
	c.mu.RLock()
	u, ok := c.cache[id]
	c.mu.RUnlock()
	if ok {
		return u, nil
	}

	// Otherwise fetch from the underlying storage.
	u, err := c.storage.Fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache[id] = u
	c.mu.Unlock()
	return u, nil
}

// implement remaining UserStorage interface, invalidating the cache...
```

### Dependencies between dependencies
Dependencies don't live in isolation. We may store _User_ data in PostgreSQL but our financial transaction data exists in a third-party service like [Stripe](https://stripe.com/). In this case, we wrap our Stripe dependency with a logical domain type — let’s call it _TransactionService_.

By adding _TransactionService_ to _UserService_ we decouple our two dependencies:

```go
type UserService struct {
	storage      UserStorage
	transactions TransactionService
}
```

Now the dependencies communicate solely through our common domain language. This means that we could swap out PostgreSQL for MySQL or switch Stripe for another payment processor without affecting other dependencies.

Don’t limit this to third party dependencies only.

## Use a shared mock subpackage
Because the dependencies are isolated from other dependencies by the domain interfaces, we can use these connection points to inject mock implementations.

```go
package mock

import (
	"context"

	"github.com/anthonycorbacho/workspace/sample/sampleapp"
)

// Verify interface compliance.
var _ sampleapp.UserStorage = (*UserStorage)(nil)

// UserStorage represents a mock implementation of sampleapp.UserStorage.
type UserStorage struct {
	FetchFn      func(ctx context.Context, id string) (*sampleapp.User, error)
	FetchInvoked bool

	// additional function implementations...
}

// Fetch invokes the mock implementation and marks the function as invoked.
func (s *UserStorage) Fetch(ctx context.Context, id string) (*sampleapp.User, error) {
	s.FetchInvoked = true
	return s.FetchFn(ctx, id)
}

// additional functions: Create(), Delete()
```

This mock injects functions into anything that uses the _sampleapp.UserStorage_ interface to validate arguments, return expected data, or inject failures.

## Main package ties together dependencies
With all these dependency packages floating around in isolation, _main_ package will be the one bringing them all together.

### Main package layout
An application may produce multiple binaries so we’ll use the Go convention of placing our _main_ package as a subdirectory of the _cmd package._ For example, the project may have a _sampleapp_ server binary but also a _sampleappctl_ client binary for managing the server from the terminal. We'll layout the main packages like this:

```
sampleapp/
    cmd/
        sampleapp/
            main.go
            grpc_user.go
        sampleappctl/
            main.go
```

### Injecting dependencies at compile time
The _main_ package is what gets to choose which dependencies to inject into which struct. Because the _main_ package simply wires up the pieces, it tends to be fairly small and trivial code (see [sample/sampleapp/cmd/sampleapp/main.go](../../sample/sampleapp/cmd/sampleapp/main.go) for the complete version):

```go
package main

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	l, err := log.New()
	if err != nil {
		return err
	}
	defer l.Close()

	// Connect to the database.
	db, err := sql.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		l.Error(ctx, "opening database", log.Error(err))
		return err
	}
	defer db.Close()

	// Create the services.
	userService := sampleapp.New(postgres.New(db))
	srv, err := newGrpcUser(userService)
	if err != nil {
		return err
	}

	// Attach to the gRPC server (and its HTTP gateway).
	foundation, err := kit.NewFoundation("sampleapp", kit.WithLogger(l))
	if err != nil {
		return err
	}
	foundation.RegisterService(func(s *grpc.Server) {
		pb.RegisterSampleAppServiceServer(s, srv)
	})
	return foundation.Serve()
}
```
