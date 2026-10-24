# Application error - Failure is our Domain

Error handling is core to the language yet the language doesn't prescribe how to handle errors. Community efforts have been made to improve and standardize error handling but many miss the centrality of errors within our application's domain. That is, your errors are as important as our `domain type`.

An error also must serve the different goals for each of its consumer roles—the application, the end-user, and the operator.

* [Why we Err](#why-we-err)
* [Who consumes our Errors](#who-consumes-our-errors)
* [Working with error effectively](#working-with-error-effectively)

## Why we Err
Errors, at their core, are simply a way of explaining why things didn't go how you wanted them to. 
Go splits errors into two groups— `panic` and `error`. Panic occurs when you don't expect something 
to go wrong such as accessing invalid memory. 
Typically, a panic is unrecoverable so our application fails catastrophically, and we simply notify an operator to fix the bug.

An `error`, on the other hand, is when we expect something could go wrong.

### Types of errors
We can divide `error` into two categories— _well-defined_ errors & _undefined errors_.

A well-defined error is one that is specified by the API such as a `sql.ErrNoRows` error returned from `sql.QueryContext()`. 
These allow us to manage our application flow because we know what to expect and can work with them on a case-by-case basis.

An undefined error is one that is undocumented by the API and therefore we are unable to thoughtfully handle it. This can occur from poor documentation but it can also occur when APIs we depend on add additional errors conditions after we've integrated our code with them.

### Error Message Style Guide and principle

 - Use `errors.New` for simple messages and `errors.NewF` for formatted messages. Make sure you're using the [kit/errors package](../../kit/errors). 
 - Error messages start with lowercase and no punctuation at the end: `unable to open file` instead of `Unable to open file.. `
 - Error messages should be actionable, e.g. `no such directory` instead of `loading service config`. 
 - Use `unable to...` instead of `failed to...`, `can't...`, or `could not...`. 
 - Messages should reflect the logical action, not merely a function name: `load service config` instead of `LoadServiceConfig`. This message should add actionable, meaningful context to the existing error message. 
 - **Errors should suggest WHY they happened**, or even better - HOW to fix them: As a maintainer, you probably have a high amount of context as to the possible causes of an error - take the time to write down some hints for users. If there's a high probability you know what's wrong, the error message itself can suggest how to fix it. But even without that certainty, think about what the user is likely to need to log out to debug the error, and offer it in the error message itself. Where relevant, Errors should display what was expected vs what was received.

## Who consumes our Errors
The tricky part about errors is that they need to be different things to different consumers of them. In any given system, we have at least 3 consumer roles— the application, the end-user, and the operator.

### Application role
Your first line of defense in error handling is our application. Our application code can recover from error states quickly and without paging anyone in the middle of the night. However, application error handling is the least flexible and it can only handle well-defined error states.

An example of this is your web browser receiving a 301 redirect code and navigating you to a new location. It's a seamless process that most users are oblivious to. It's able to do this because the HTTP specification has well-defined error codes.

### End-user role
If our application is unable to handle the error condition then hopefully your end-user can resolve the issue. Your end-user can see an error state such as `"You reached our quota, please retry in 1 hour"` and is flexible enough to resolve it (eg: wait one hour and retry).

Unlike the application role, the end-user needs a human-readable message that can provide context to help them resolve the error.

These users are still limited to well-defined errors since revealing undefined errors could compromise the security of your system. For example, a postgres error may detail query or schema information that could be used by an attacker. When confronted with an undefined error, it may be appropriate to simply tell the user to contact technical support.

### Operator role
Finally, the last line of defense is the system operator which may be a developer or an operations person. These people understand the details of the system and can work with any kind of error.

In this role, you typically want to see as much information as possible. In addition to the error code and human-readable message, a logical stack trace can help the operator understand the program flow.

## Working with error effectively
Our Go project layout is generally composed into 3 distinct components

1. Handler (HTTP, GRPC)
2. Root (business domain)
3. Dependency (database, 3rd party client)

Each layer has a different responsibility and errors should be handled differently.

For expected domain errors (eg: not found, unauthorized, etc) it is better to define them at the root layer and utilize them in the dependency layer. This will help to handle efficiently business expected errors without the need to refine and translate them on every dependency layer (eg: grpc client, mock)

### Error at Root layer
The root layer is the brain of the application. The main goal of the error is to be abstract from the dependency. The root layer should not know about the implementation detail of the dependency, and this also applies to the error, errors are bound to the dependency as they are part of the API.

This is why the Root layer usually has a common error file that groups the expected error type

```go
// List of expected domain error that are applicable
// regardless of the implementation detail of the dependency layout.
const (
  ErrNotFound  = Error("not found")
// ...
)

// Error represents a domain error.
type Error string

// Error returns the error message.
func (e Error) Error() string {
  return string(e)
}
```

in the caller function of the service, we can also wrap the error to enable more data context into the error call chain, giving the error enough information to help operator debugging.

```go
func (srv *MyService) Fetch(ctx context.Context, id string) (&Struct, error) {
  // ... perform validation and setup tracing

  d, err := svc.client.Fetch(ctx, id)
  if err != nil {
    return nil, errors.Wrap(err, "srv.fetch")
  }

  d2, err := svc.c2.FetchMetadata(ctx, id)
  if err != nil {
    return nil, errors.Wrap(err, "myservice.fetch")
  }

// ...

}
```

### Error at Dependency layer
In the application layout, this will be the lower level, basically, we will wrap the error with the internal action this layer is doing to give enough error context.

```go
func (c *Client) Fetch(ctx context.Context, id string) (&Struct, error) {
  d, err := c.Select(ctx, id)
  if err != nil {
    // case of business defined error
    if err == sql.ErrNoRows {
      return nil, myapp.ErrNotFound
    }
    // case of undefined error
    return nil, errors.Warp(error, "sql.client.fetch")
  }

  return convert(d), nil
}
```

### Error at Handler layer
The handler layer returns the errors as is: they are coded errors created by the lower layers (see below),
the kit gRPC server sends them to the client and turns any other error into an `Internal` error.

```go
func (h *grpcUser) Fetch(ctx context.Context, req *pb.FetchRequest) (*pb.FetchResponse, error) {
	user, err := h.service.Fetch(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	// ...
}
```

## Coded errors
Coded errors are defined in a single catalog, [kit/errors/errors.defs.yaml](../../kit/errors/errors.defs.yaml),
the constructors (`kit/errors/error.gen.go`) and the documentation (`kit/errors/docs`) are generated with `make errgen`.

```yaml
# Reserved code blocks per domain:
#   - 00XXX: kit, generic errors used by the kit.
#   - 01XXX: sample, errors of the sample application.
errors:
  - name: SampleUserNotFound      # constructor name, errors.SampleUserNotFound(id)
    code: ERR_01001               # ERR_<5 digits>, unique, in the block of the domain
    kind: NOT_FOUND               # category of the error, mapped to a gRPC code and an HTTP status (404)
    message: "user {id} not found" # client facing message
    vars:                          # constructor arguments, sent in the details
      - name: id
        type: string
```

- **The code is part of the API**: clients and support rely on it, a released code must never change nor be reused
  by another error. The CI rejects a catalog changing or reusing the code of an error on the main branch.
- **Each domain owns a block of codes** (eg: `01XXX` for the sample), add a new block for a new domain.
- **The kind is transport free**: `kit/errors` doesn't depend on gRPC, the transports map the kind to their status
  (`kit/grpc` to a gRPC code, the gateway to an HTTP status). The kinds follow the gRPC canonical codes, which are
  precise (eg: `ALREADY_EXISTS` vs `ABORTED` for a conflict).
- **Internal information never reaches the client**: a var with `internal: true` and the cause (`WithCause`) are
  only part of the error string (logs) and of the error chain (`errors.Is`, `errors.As`).

### Errors by layer
The coded errors are created where the failure is understood and bubble up:

| Layer | Responsibility |
|---|---|
| Adapter (eg: `postgres/`, `inmem/`) | Translates the dependency errors: a missing row becomes `errors.SampleUserNotFound(id)`, a unique constraint violation `errors.SampleUserAlreadyExists(name)`, any other error is returned with context (`errors.Wrap`) and becomes `Internal`. The dependency error is kept with `WithCause`, never in the message. |
| Root package (domain, service) | Validates the domain rules (`InvalidRequest` with violations) and passes the errors through. It re-maps an error only when the context changes its meaning, eg: a user not found while creating an order is an invalid request, not a not found order. |
| Handler (`cmd/<service>/grpc_*.go`) | Returns the errors as is. |

The root package depends on `kit/errors` only, never on a transport: the CI (depguard) rejects gRPC imports in
`kit/errors` and in the root package of the services.

To check an error, use its code:

```go
if errors.HasCode(err, errors.SampleUserNotFoundCode) {
	// ...
}
```

### On the wire
A coded error travels as a gRPC status (the code of its kind and its message) with an `errdetails.v1.Error`
detail (code, message, details). Through the HTTP gateway, the gRPC code is mapped to an HTTP status and the body is:

```json
{
  "code": "ERR_01001",
  "message": "user 1234 not found",
  "details": { "id": "1234" },
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
}
```

The kit gRPC server and client handle the conversions (`kit/grpc.Status`, `kit/grpc.FromStatus`):
- a plain error returned by a handler becomes an `Internal` error (`ERR_00000`), it is logged but its message is not sent.
- an invalid request (protovalidate rules) is rejected with `InvalidRequest` (`ERR_00001`), the violations are in the details.
- a panic is recovered as an `Internal` error.
- the kit gRPC client returns coded errors: `errors.HasCode` works across services, and a coded error returned as is
  by a service reaches the end client with its code.

### Adding an error
1. Add the error at the end of its domain block in `kit/errors/errors.defs.yaml` with the next code of the block.
2. Run `make errgen`, it validates the catalog (unique codes and names, valid kind, placeholders...) and
   regenerates `kit/errors/error.gen.go` and `kit/errors/docs`.
3. Commit the catalog and the generated files, the CI fails if they are not up to date.
