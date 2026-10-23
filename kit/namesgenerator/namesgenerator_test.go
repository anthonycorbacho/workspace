package namesgenerator

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomName(t *testing.T) {
	for range 1000 {
		name := RandomName()

		parts := strings.Split(name, "_")
		require.Len(t, parts, 2, "name %q should be formatted as adjective_surname", name)
		assert.True(t, slices.Contains(_left[:], parts[0]), "unknown adjective %q", parts[0])
		assert.True(t, slices.Contains(_right[:], parts[1]), "unknown surname %q", parts[1])
	}
}

func TestRandomNameDistinct(t *testing.T) {
	seen := make(map[string]struct{})
	for range 100 {
		seen[RandomName()] = struct{}{}
	}
	// With thousands of combinations, 100 draws must not all collide.
	assert.Greater(t, len(seen), 1)
}

func TestWordLists(t *testing.T) {
	tests := []struct {
		name  string
		words []string
	}{
		{name: "left", words: _left[:]},
		{name: "right", words: _right[:]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEmpty(t, tt.words)
			seen := make(map[string]struct{}, len(tt.words))
			for _, w := range tt.words {
				assert.NotEmpty(t, w)
				assert.NotContains(t, w, "_", "word %q would break the name format", w)
				_, dup := seen[w]
				assert.False(t, dup, "duplicated word %q", w)
				seen[w] = struct{}{}
			}
		})
	}
}
