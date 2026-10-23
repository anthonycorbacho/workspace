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

type user struct {
	Name  string   `validate:"required"`
	Email string   `validate:"required,email"`
	Age   int      `validate:"gte=0,lte=130"`
	Role  string   `validate:"oneof=admin member"`
	Tags  []string `validate:"max=2,dive,required"`
}

func TestStruct(t *testing.T) {
	valid := user{Name: "john", Email: "john@example.com", Age: 30, Role: "admin"}

	tests := []struct {
		name       string
		modify     func(u *user)
		wantFields []string
	}{
		{name: "valid", modify: func(*user) {}},
		{name: "valid with tags", modify: func(u *user) { u.Tags = []string{"a", "b"} }},
		{name: "valid zero age", modify: func(u *user) { u.Age = 0 }},
		{name: "missing name", modify: func(u *user) { u.Name = "" }, wantFields: []string{"Name"}},
		{name: "invalid email", modify: func(u *user) { u.Email = "not-an-email" }, wantFields: []string{"Email"}},
		{name: "negative age", modify: func(u *user) { u.Age = -1 }, wantFields: []string{"Age"}},
		{name: "age too high", modify: func(u *user) { u.Age = 131 }, wantFields: []string{"Age"}},
		{name: "unknown role", modify: func(u *user) { u.Role = "owner" }, wantFields: []string{"Role"}},
		{name: "too many tags", modify: func(u *user) { u.Tags = []string{"a", "b", "c"} }, wantFields: []string{"Tags"}},
		{name: "empty tag", modify: func(u *user) { u.Tags = []string{"a", ""} }, wantFields: []string{"Tags[1]"}},
		{name: "empty struct", modify: func(u *user) { *u = user{} }, wantFields: []string{"Name", "Email", "Role"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := valid
			tt.modify(&u)

			err := New().Struct(u)
			if len(tt.wantFields) == 0 {
				assert.NoError(t, err)
				return
			}

			var validationErrs ValidationErrors
			require.True(t, stdErrors.As(err, &validationErrs))
			fields := make([]string, 0, len(validationErrs))
			for _, fe := range validationErrs {
				fields = append(fields, fe.Field())
			}
			assert.Equal(t, tt.wantFields, fields)
		})
	}
}

func TestVar(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		tag     string
		wantErr bool
	}{
		{name: "valid email", value: "john@example.com", tag: "email"},
		{name: "invalid email", value: "john", tag: "email", wantErr: true},
		{name: "required empty", value: "", tag: "required", wantErr: true},
		{name: "required zero int", value: 0, tag: "required", wantErr: true},
		{name: "min length ok", value: "abc", tag: "min=3"},
		{name: "min length ko", value: "ab", tag: "min=3", wantErr: true},
		{name: "empty tag", value: "anything", tag: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New().Var(tt.value, tt.tag)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

type privateHolder struct {
	name string `validate:"required"` //nolint:unused
}

func TestWithPrivateFieldValidation(t *testing.T) {
	value := privateHolder{}

	err := New().Struct(value)
	assert.NoError(t, err)

	err = New(WithPrivateFieldValidation()).Struct(value)
	assert.Error(t, err)

	var validationErrs ValidationErrors
	assert.True(t, stdErrors.As(err, &validationErrs))

	err = New(WithPrivateFieldValidation()).Struct(privateHolder{name: "john"})
	assert.NoError(t, err)
}

func TestRegisterValidation(t *testing.T) {
	type holder struct {
		Code string `validate:"is_kit"`
	}

	v := New()
	require.NoError(t, v.RegisterValidation("is_kit", Func(func(fl FieldLevel) bool {
		return fl.Field().String() == "kit"
	})))

	assert.NoError(t, v.Struct(holder{Code: "kit"}))
	assert.Error(t, v.Struct(holder{Code: "other"}))
}

func TestInvalidValidationError(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{name: "nil", input: nil},
		{name: "not a struct", input: 42},
		{name: "nil struct pointer", input: (*simple)(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New().Struct(tt.input)
			require.Error(t, err)

			var invalidErr *InvalidValidationError
			assert.True(t, stdErrors.As(err, &invalidErr))
		})
	}
}
