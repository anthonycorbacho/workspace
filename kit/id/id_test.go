package id

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// _idFormat is the xid base32 hex (lower case) representation: 20 characters.
var _idFormat = regexp.MustCompile(`^[0-9a-v]{20}$`)

func TestNew(t *testing.T) {
	id := New()
	assert.NotEmpty(t, id)
	assert.Regexp(t, _idFormat, id)
}

func TestNewUnique(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for range n {
		id := New()
		_, dup := seen[id]
		assert.False(t, dup, "duplicated id %s", id)
		seen[id] = struct{}{}
	}
	assert.Len(t, seen, n)
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   *regexp.Regexp
	}{
		{name: "with prefix", prefix: "test", want: regexp.MustCompile(`^test/[0-9a-v]{20}$`)},
		{name: "with nested prefix", prefix: "org/user", want: regexp.MustCompile(`^org/user/[0-9a-v]{20}$`)},
		{name: "empty prefix", prefix: "", want: _idFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := NewGenerator(tt.prefix)
			id := generator.Generate()
			assert.True(t, strings.HasPrefix(id, tt.prefix))
			assert.Regexp(t, tt.want, id)
			assert.NotEqual(t, id, generator.Generate())
		})
	}
}
