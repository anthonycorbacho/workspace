package log

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type stringer string

func (s stringer) String() string { return string(s) }

func TestFields(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		field    Field
		wantKey  string
		wantType FieldType
	}{
		{name: "string", field: String("k", "v"), wantKey: "k", wantType: StringType},
		{name: "strings", field: Strings("k", []string{"a", "b"}), wantKey: "k", wantType: ArrayMarshalerType},
		{name: "byte string", field: ByteString("k", []byte("v")), wantKey: "k", wantType: ByteStringType},
		{name: "stringer", field: Stringer("k", stringer("v")), wantKey: "k", wantType: StringerType},
		{name: "bool", field: Bool("k", true), wantKey: "k", wantType: BoolType},
		{name: "int", field: Int("k", 1), wantKey: "k", wantType: Int64Type},
		{name: "int32", field: Int32("k", 1), wantKey: "k", wantType: Int32Type},
		{name: "int64", field: Int64("k", 1), wantKey: "k", wantType: Int64Type},
		{name: "uint", field: Uint("k", 1), wantKey: "k", wantType: Uint64Type},
		{name: "uint32", field: Uint32("k", 1), wantKey: "k", wantType: Uint32Type},
		{name: "uint64", field: Uint64("k", 1), wantKey: "k", wantType: Uint64Type},
		{name: "float32", field: Float32("k", 1.5), wantKey: "k", wantType: Float32Type},
		{name: "float64", field: Float64("k", 1.5), wantKey: "k", wantType: Float64Type},
		{name: "error", field: Error(errors.New("boom")), wantKey: "error", wantType: ErrorType},
		{name: "nil error is skipped", field: Error(nil), wantKey: "", wantType: SkipType},
		{name: "duration", field: Duration("k", time.Second), wantKey: "k", wantType: DurationType},
		{name: "time", field: Time("k", now), wantKey: "k", wantType: TimeType},
		{name: "any string", field: Any("k", "v"), wantKey: "k", wantType: StringType},
		{name: "any struct", field: Any("k", struct{ A int }{A: 1}), wantKey: "k", wantType: ReflectType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantKey, tt.field.Key)
			assert.Equal(t, tt.wantType, FieldType(tt.field.Type))
		})
	}
}
