package validator

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type simple struct {
	Name string `validate:"required"`
}

type requiredStructHolder struct {
	Payload payloadStruct `validate:"required"`
}

type payloadStruct struct {
	Count int
}

func TestNew(t *testing.T) {
	v := New()
	require.NotNil(t, v)

	err := v.Struct(simple{})
	assert.Error(t, err)

	var validationErrs ValidationErrors
	assert.True(t, stdErrors.As(err, &validationErrs))
}

func TestWithRequiredStructEnabled(t *testing.T) {
	value := requiredStructHolder{}

	err := New().Struct(value)
	assert.NoError(t, err)

	err = New(WithRequiredStructEnabled()).Struct(value)
	assert.Error(t, err)

	var validationErrs ValidationErrors
	assert.True(t, stdErrors.As(err, &validationErrs))
}

func TestInvalidValidationError(t *testing.T) {
	err := New().Struct(nil)
	require.Error(t, err)

	var invalidErr *InvalidValidationError
	assert.True(t, stdErrors.As(err, &invalidErr))
}
