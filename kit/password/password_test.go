package password

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
