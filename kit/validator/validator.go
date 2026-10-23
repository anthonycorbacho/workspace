package validator

import v10 "github.com/go-playground/validator/v10"

type (
	// Validate validates structs and individual fields.
	Validate = v10.Validate
	// Option configures the validator behavior.
	Option = v10.Option

	// Func is a custom field-level validation function.
	Func = v10.Func
	// FuncCtx is a context-aware custom field-level validation function.
	FuncCtx = v10.FuncCtx
	// FieldLevel contains information and helper methods for field-level validation.
	FieldLevel = v10.FieldLevel
	// StructLevelFunc is a custom struct-level validation function.
	StructLevelFunc = v10.StructLevelFunc
	// StructLevelFuncCtx is a context-aware custom struct-level validation function.
	StructLevelFuncCtx = v10.StructLevelFuncCtx
	// StructLevel contains information and helper methods for struct-level validation.
	StructLevel = v10.StructLevel
	// TagNameFunc extracts alternate field names used by validation errors.
	TagNameFunc = v10.TagNameFunc
	// FilterFunc decides whether a namespace should be filtered during partial validation.
	FilterFunc = v10.FilterFunc
	// CustomTypeFunc allows overriding values for custom type handling.
	CustomTypeFunc = v10.CustomTypeFunc

	// FieldError describes a single field validation failure.
	FieldError = v10.FieldError
	// ValidationErrors is a collection of field validation failures.
	ValidationErrors = v10.ValidationErrors
	// ValidationErrorsTranslations maps field names to translated messages.
	ValidationErrorsTranslations = v10.ValidationErrorsTranslations
	// InvalidValidationError indicates invalid usage/input of the validator.
	InvalidValidationError = v10.InvalidValidationError

	// RegisterTranslationsFunc registers translation messages.
	RegisterTranslationsFunc = v10.RegisterTranslationsFunc
	// TranslationFunc translates a field error.
	TranslationFunc = v10.TranslationFunc
)

// New creates a new validator instance.
func New(options ...Option) *Validate {
	return v10.New(options...)
}

// WithPrivateFieldValidation enables validation of private fields.
func WithPrivateFieldValidation() Option {
	return v10.WithPrivateFieldValidation()
}

// WithRequiredStructEnabled enables required validation for non-pointer structs.
func WithRequiredStructEnabled() Option {
	return v10.WithRequiredStructEnabled()
}
