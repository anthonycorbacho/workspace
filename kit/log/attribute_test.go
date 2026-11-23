package log

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildAttributes(t *testing.T) {
	attr := buildAttributes("/path/to/file.go:42", []Field{
		String("a", "1"),
		String("b", "2"),
		String("a", "3"),
		Error(nil),
	})

	assert.Equal(t, "Attributes", attr.Key)
	got := make(map[string]any)
	for _, a := range attr.Value.Group() {
		got[a.Key] = a.Value.Any()
	}
	assert.Equal(t, map[string]any{
		"caller.full_path": "/path/to/file.go:42",
		"a":                "3",
		"b":                "2",
	}, got)
}
