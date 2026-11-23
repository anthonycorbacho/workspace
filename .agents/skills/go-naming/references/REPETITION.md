# Avoiding Repetition

This reference covers how to avoid redundant naming in Go by considering the context
where names appear—package, receiver type, and surrounding code.

## Package vs. Exported Symbol

> **Advisory**: Don't repeat package name in exported symbols.

```go
// Bad - repetitive at call site
package widget
func NewWidget() *Widget           // widget.NewWidget()
func NewWidgetWithName(n string)   // widget.NewWidgetWithName()

// Good - concise at call site
package widget
func New() *Widget                 // widget.New()
func NewWithName(n string) *Widget // widget.NewWithName()
```

```go
// Bad
package db
func LoadFromDatabase() error      // db.LoadFromDatabase()

// Good
package db
func Load() error                  // db.Load()
```

## Method vs. Receiver Type

> **Advisory**: Don't repeat receiver type in method name.

```go
// Bad
func (c *Config) WriteConfigTo(w io.Writer) error
func (p *Project) ProjectName() string

// Good
func (c *Config) WriteTo(w io.Writer) error
func (p *Project) Name() string
```

## Context vs. Local Names

> **Advisory**: Omit information already clear from context.

```go
// Bad - in package "ads/targeting/revenue/reporting"
type AdsTargetingRevenueReport struct{}

// Good
type Report struct{}
```

```go
// Bad - in package "sqldb"
type DBConnection struct{}

// Good
type Connection struct{}
```

## Complete Example

```go
// Bad - excessive repetition
func (db *DB) UserCount() (userCount int, err error) {
    var userCountInt64 int64
    if dbLoadError := db.LoadFromDatabase("count(distinct users)", &userCountInt64); dbLoadError != nil {
        return 0, fmt.Errorf("failed to load user count: %s", dbLoadError)
    }
    userCount = int(userCountInt64)
    return userCount, nil
}

// Good - clear and concise
func (db *DB) UserCount() (int, error) {
    var count int64
    if err := db.Load("count(distinct users)", &count); err != nil {
        return 0, fmt.Errorf("failed to load user count: %s", err)
    }
    return int(count), nil
}
```
