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
The handler layer will return to the end-user the error formatted for him to understand and take action (call support, update a setting in the dashboard, or other actions).

```go
func (h *myGrpcHandler) Fetch(ctx context.Context, req v1.Request) (*v1.Response, error) {
// ... perform validation and setup tracing

d, err := h.myService.Fetch(ctx, req.ID)
if err != nil {
// ... log the error for the operator

    // defined error
    if errors.Is(err, myapp.ErrNotFound) {
      return nil,
        errors.Status(
          codes.NotFound,
          "couldnt get data because not found",
          &errdetails.ErrorInfo{
			Reason: "NOT_FOUND",
			Metadata: map[string]string{
				"id":   string(req.ID),
			},
		})
    }
    // Undefined error
    return nil,
        errors.Status(
          codes.Internal,
          err,
          &errdetails.ErrorInfo{
			Reason: "SERVER_ERROR",
			Metadata: map[string]string{
				"id":   string(req.ID),
			},
		})
}

// ...
}
```

