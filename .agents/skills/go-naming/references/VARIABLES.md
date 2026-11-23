# Variable Names

This reference provides detailed guidance on naming variables in Go, covering scope-based
naming, single-letter conventions, and avoiding type redundancy.

## Length Proportional to Scope

> **Advisory**: Short names for small scopes, longer names for large scopes.

| Scope        | Lines  | Name Length |
|--------------|--------|-------------|
| Small        | 1-7    | 1-2 chars   |
| Medium       | 8-15   | short word  |
| Large        | 15-25  | descriptive |
| Very large   | 25+    | full words  |

```go
// Good - short scope, short name
for i := 0; i < len(items); i++ {
    process(items[i])
}

// Good - larger scope, clearer name
func processOrders(orders []*Order) error {
    pendingOrders := filterPending(orders)
    // ... 20+ lines of processing ...
    return nil
}
```

## Single-Letter Variables

> **Advisory**: Use single letters only when meaning is obvious.

Appropriate uses:
- Loop indices: `i`, `j`, `k`
- Coordinates: `x`, `y`, `z`
- Receivers: one or two letters
- Common types: `r` for `io.Reader`, `w` for `io.Writer`
- Short loops: `for _, n := range nodes`

```go
// Good - familiar conventions
func Copy(w io.Writer, r io.Reader) (int64, error)

for i, v := range values {
    process(v)
}

// Bad - unclear single letters
func Process(a, b, c string) error  // what are a, b, c?
```

## Avoid Type in Variable Name

> **Advisory**: Don't include the type in the variable name.

| Repetitive (Bad)             | Better               |
|------------------------------|----------------------|
| `var numUsers int`           | `var users int`      |
| `var nameString string`      | `var name string`    |
| `var primaryProject *Project`| `var primary *Project`|
| `var userSlice []User`       | `var users []User`   |

When disambiguating multiple forms, use meaningful qualifiers:

```go
// Good - meaningful distinction
limitRaw := r.FormValue("limit")
limit, err := strconv.Atoi(limitRaw)

// Also good
limitStr := r.FormValue("limit")
limit, err := strconv.Atoi(limitStr)
```

## Prefix Unexported Globals with _

> **Source**: Uber Go Style Guide

Prefix unexported top-level `var`s and `const`s with `_` to clarify when they
are used that they are global symbols.

**Rationale**: Top-level variables and constants have package scope. Using a
generic name makes it easy to accidentally shadow the value in a different file.

```go
// Bad - hard to distinguish from local variables
const (
    defaultPort = 8080
    defaultUser = "user"
)

func Bar() {
    defaultPort := 9090  // shadows global, no compile error
    fmt.Println("Default port", defaultPort)
}
```

```go
// Good - clearly global
const (
    _defaultPort = 8080
    _defaultUser = "user"
)
```

**Exception**: Unexported error values use the `err` prefix without underscore:

```go
var errUserNotFound = errors.New("user not found")
var errInvalidInput = errors.New("invalid input")
```
