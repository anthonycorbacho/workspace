package id

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	id := New()
	assert.NotEmpty(t, id)
}

func TestGenerate(t *testing.T) {
	generator := NewGenerator("test")
	id := generator.Generate()
	assert.True(t, strings.HasPrefix(id, "test/"))
}
