package id

import (
	"fmt"
	"time"

	"github.com/rs/xid"
)

// New generates a globally unique ID
func New() string {
	return xid.NewWithTime(time.Now().UTC()).String()
}

// Generator will generate prefixed ID.
type Generator struct {
	prefix string
}

// NewGenerator creates a new ID generator with prefix.
// the prefix format will follow the partition convention as follows: <PREFIX>/<GLOBALLY_UNIQUE_ID>
func NewGenerator(prefix string) *Generator {
	return &Generator{prefix: prefix}
}

// Generate generates a prefixed globally unique ID.
func (g *Generator) Generate() string {
	id := New()
	if len(g.prefix) == 0 {
		return id
	}
	return fmt.Sprintf("%s/%s", g.prefix, id)
}
