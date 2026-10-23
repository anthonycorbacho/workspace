package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	pwd, err := Generate(8, 3, 2)
	assert.NoError(t, err)
	assert.NotEmpty(t, pwd)
	assert.Equal(t, 8, len(pwd))

	pwd, err = Generate(20, 4, 4)
	assert.NoError(t, err)
	assert.NotEmpty(t, pwd)
	assert.Equal(t, 20, len(pwd))
}

func TestGenerateConstraints(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		digits  int
		symbols int
		wantErr bool
	}{
		{name: "minimum length letters only", length: 8},
		{name: "digits and symbols", length: 16, digits: 4, symbols: 4},
		{name: "digits only", length: 10, digits: 10},
		{name: "symbols only", length: 10, symbols: 10},
		{name: "digits and symbols fill length", length: 8, digits: 4, symbols: 4},
		{name: "long password", length: 256, digits: 50, symbols: 50},
		{name: "length below minimum", length: 7, wantErr: true},
		{name: "zero length", length: 0, wantErr: true},
		{name: "negative length", length: -1, wantErr: true},
		{name: "digits exceed length", length: 8, digits: 9, wantErr: true},
		{name: "symbols exceed length", length: 8, symbols: 9, wantErr: true},
		{name: "digits and symbols exceed length", length: 8, digits: 5, symbols: 4, wantErr: true},
		// negative counts are invalid, they must not change the password length.
		{name: "negative digits", length: 8, digits: -2, wantErr: true},
		{name: "negative symbols", length: 8, symbols: -2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pwd, err := Generate(tt.length, tt.digits, tt.symbols)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, pwd)
				return
			}
			require.NoError(t, err)
			require.Len(t, pwd, tt.length)

			var letters, digits, symbols int
			for _, r := range pwd {
				switch {
				case strings.ContainsRune(_letters, r):
					letters++
				case strings.ContainsRune(_digits, r):
					digits++
				case strings.ContainsRune(_symbols, r):
					symbols++
				default:
					assert.Failf(t, "unexpected character", "%q in %q", r, pwd)
				}
			}
			assert.Equal(t, tt.digits, digits)
			assert.Equal(t, tt.symbols, symbols)
			assert.Equal(t, tt.length-tt.digits-tt.symbols, letters)
		})
	}
}

func TestGenerateRandom(t *testing.T) {
	seen := make(map[string]struct{})
	for range 100 {
		pwd, err := Generate(16, 4, 4)
		require.NoError(t, err)
		seen[pwd] = struct{}{}
	}
	assert.Len(t, seen, 100)
}
