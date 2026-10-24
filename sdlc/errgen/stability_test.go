package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckStability(t *testing.T) {
	base := catalogYAML(`
  - {name: A, code: ERR_00001, kind: INTERNAL, message: a}
  - {name: B, code: ERR_00002, kind: INTERNAL, message: b}
`)

	tests := []struct {
		name    string
		current string
		want    []string
	}{
		{
			name:    "unchanged",
			current: base,
		},
		{
			name: "new and removed errors",
			current: catalogYAML(`
  - {name: A, code: ERR_00001, kind: NOT_FOUND, message: a changed}
  - {name: C, code: ERR_00003, kind: INTERNAL, message: c}
`),
		},
		{
			name: "changed code",
			current: catalogYAML(`
  - {name: A, code: ERR_00011, kind: INTERNAL, message: a}
  - {name: B, code: ERR_00002, kind: INTERNAL, message: b}
`),
			want: []string{"errors.defs.yaml:5: A: code changed from ERR_00001 to ERR_00011, a released code must never change"},
		},
		{
			name: "reused code",
			current: catalogYAML(`
  - {name: A, code: ERR_00001, kind: INTERNAL, message: a}
  - {name: C, code: ERR_00002, kind: INTERNAL, message: c}
`),
			want: []string{"errors.defs.yaml:6: C: code ERR_00002 was used by B in base.yaml, a released code must never be reused"},
		},
		{
			name: "swapped codes",
			current: catalogYAML(`
  - {name: A, code: ERR_00002, kind: INTERNAL, message: a}
  - {name: B, code: ERR_00001, kind: INTERNAL, message: b}
`),
			want: []string{
				"errors.defs.yaml:5: A: code changed from ERR_00001 to ERR_00002, a released code must never change",
				"errors.defs.yaml:5: A: code ERR_00002 was used by B in base.yaml, a released code must never be reused",
				"errors.defs.yaml:6: B: code changed from ERR_00002 to ERR_00001, a released code must never change",
				"errors.defs.yaml:6: B: code ERR_00001 was used by A in base.yaml, a released code must never be reused",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := parseCatalog(t, base)
			require.NoError(t, err)
			b.File = "base.yaml"
			c, err := parseCatalog(t, tt.current)
			require.NoError(t, err)

			err = checkStability(b, c)
			if len(tt.want) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ElementsMatch(t, tt.want, splitLines(err.Error()))
		})
	}
}
