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
The application has a logical, high-level language that describes how data and processes interact. This is the domain. If we have an e-commerce application the domain involves things like customers, accounts, charging credit cards, and handling inventory. If we are Facebook then the domain is users, likes, & relationships. It is the stuff that doesn't depend on the underlying technology.

We will place the domain types in the root package. This package only contains simple data types like a _User_ struct for holding user data or a _UserStorage/UserRepository_ interface for fetching or saving user data.

It may look something like:

```go
package sampleapp

type User struct {
	ID      int
	Name    string
}

type UserService struct {
	storage UserStorage
}

type UserStorage interface {
	Get(id int) (*User, error)
	List() ([]*User, error)
	Create(u *User) error
	Delete(id int) error
}
```

This makes the root package extremely simple. We may also include types that perform actions but only if they solely depend on other domain types. For example, you could have a type that polls the _UserService_ periodically. However, it should not call out to external services or save to a database. That is an implementation detail.

The root package should not depend on any other package in the application

## Group subpackages by dependency
If the root package is not allowed to have external dependencies then we must push those dependencies to subpackages. In this approach to package layout, subpackages exist as an adapter between our domain and the implementation.

For example, the _UserService_ might be backed by PostgreSQL. We can introduce a postgres subpackage in the application that provides a postgres_.UserStorage_ implementation:

```go
package postgres

import (
	"database/sql"

	"github.com/anthonycorbacho/workspace/sample/sampleapp"
	_ "github.com/lib/pq"
)

// UserStorage represents a PostgreSQL implementation of sampleapp.UserStorage.
type UserStorage struct {
	DB *sql.DB
}

// Get returns a user for a given id.
func (s *UserStorage) Get(id int) (*sampleapp.User, error) {
	var u sampleapp.User
	row := db.QueryRow(`SELECT id, name FROM users WHERE id = $1`, id)
	if err := row.Scan(&u.ID, &u.Name); err != nil {
		return nil, err
	}
	return &u, nil
}

// implement remaining sampleapp.UserStorage interface...
```

This isolates the PostgreSQL dependency which simplifies testing and provides an easy way to migrate to another database in the future. It can be used as a pluggable architecture if we decide to support other database implementations such as MySQL.

It also gives you a way to layer implementations. Perhaps we want to hold an in-memory, [LRU cache](https://en.wikipedia.org/wiki/Cache_algorithms) in front of PostgreSQL. We can add a _UserCache_ that implements _UserStorage_ which can wrap the PostgreSQL implementation:

```go
package sampleapp

// UserCache wraps a UserStorage to provide an in-memory cache.
type UserCache struct {
  cache   map[int]*User
  service UserStorage
}

// NewUserCache returns a new read-through cache for storage.
func NewUserCache(storage UserStorage) *UserCache {
  return &UserCache{
  cache: make(map[int]*User),
  storage: storage,
  }
}

// Get returns a user for a given id.
// Returns the cached instance if available.
func (c *UserCache) Get(id int) (*User, error) {
  // Check the local cache first.
  if u := c.cache[id]]; u != nil {
    return u, nil
  }

  // Otherwise fetch from the underlying service.
  u, err := c.storage.Get(id)
  if err != nil {
    return nil, err
  } else if u != nil {
    c.cache[id] = u
  }
  return u, err
}
```

### Dependencies between dependencies
Dependencies don't live in isolation. We may store _User_ data in PostgreSQL but our financial transaction data exists in a third-party service like [Stripe](https://stripe.com/). In this case, we wrap our Stripe dependency with a logical domain type — let’s call it _TransactionService_.

By adding _TransactionService_ to _UserService_ we decouple our two dependencies:

```go
type UserService struct {
	Storage            sampleapp.UserStorage
	TransactionService sampleapp.TransactionService
}
```

Now the dependencies communicate solely through our common domain language. This means that we could swap out PostgreSQL for MySQL or switch Stripe for another payment processor without affecting other dependencies.

Don’t limit this to third party dependencies only.

## Use a shared mock subpackage
Because the dependencies are isolated from other dependencies by the domain interfaces, we can use these connection points to inject mock implementations.

```go
package mock

import "github.com/anthonycorbacho/workspace/sample/sampleapp"

// UserStorage represents a mock implementation of sampleapp.UserStorage.
type UserStorage struct {
  UserFn      func(id int) (*sampleapp.User, error)
  UserInvoked bool

  UsersFn     func() ([]*sampleapp.User, error)
  UsersInvoked bool

  // additional function implementations...
}

// Get invokes the mock implementation and marks the function as invoked.
func (s *UserStorage) Get(id int) (*sampleapp.User, error) {
  s.UserInvoked = true
  return s.UserFn(id)
}

// additional functions: List(), Create(), Delete()
```

This mock inject functions into anything that uses the _sampleapp.UserStorage_ interface to validate arguments, return expected data, or inject failures.

## Main package ties together dependencies
With all these dependency packages floating around in isolation, _main_ package will be the one bringing them all together.

### Main package layout
An application may produce multiple binaries so we’ll use the Go convention of placing our _main_ package as a subdirectory of the _cmd package._ For example, the project may have a _sampleapp_ server binary but also a _sampleappctl_ client binary for managing the server from the terminal. We'll layout the main packages like this:

```
sampleapp/
    cmd/
        sampleapp/
            main.go
            http_user.go
            grpc_user.go
        sampleappctl/
            main.go
```

### Injecting dependencies at compile time
The _main_ package is what gets to choose which dependencies to inject into which struct. Because the _main_ package simply wires up the pieces, it tends to be fairly small and trivial code:

```go
package main

import (
	"log"
	"os"
	
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
	"github.com/anthonycorbacho/workspace/sample/sampleapp/postgres"
)

func main() {
	// Connect to database.
	db, err := postgres.Open(os.Getenv("DB"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create services.
	storage := &postgres.UserStorage{DB: db}
	
	userService := sampleapp.UserService{storage: storage}

	// Attach to HTTP handler.
	handler := UserHandler{service: userService}
	
	// start http server...
}
```
