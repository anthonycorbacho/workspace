package log

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type panicStringer struct{}

func (*panicStringer) String() string { panic("boom") }

type userObject struct{ name string }

func (u userObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", u.name)
	return nil
}

func TestAttributesMarshalLogObject(t *testing.T) {
	now := time.Unix(0, 1700000000000000000)
	tests := []struct {
		name    string
		field   Field
		want    map[string]any
		wantErr bool
	}{
		{name: "string", field: zap.String("k", "v"), want: map[string]any{"k": "v"}},
		{name: "strings", field: zap.Strings("k", []string{"a", "b"}), want: map[string]any{"k": []any{"a", "b"}}},
		{name: "object", field: zap.Object("k", userObject{name: "bob"}), want: map[string]any{"k": map[string]any{"name": "bob"}}},
		{name: "inline", field: zap.Inline(userObject{name: "bob"}), want: map[string]any{"name": "bob"}},
		{name: "binary", field: zap.Binary("k", []byte{1, 2}), want: map[string]any{"k": []byte{1, 2}}},
		{name: "bool true", field: zap.Bool("k", true), want: map[string]any{"k": true}},
		{name: "bool false", field: zap.Bool("k", false), want: map[string]any{"k": false}},
		{name: "byte string", field: zap.ByteString("k", []byte("v")), want: map[string]any{"k": "v"}},
		{name: "complex128", field: zap.Complex128("k", 1+2i), want: map[string]any{"k": complex128(1 + 2i)}},
		{name: "complex64", field: zap.Complex64("k", 1+2i), want: map[string]any{"k": complex64(1 + 2i)}},
		{name: "duration", field: zap.Duration("k", time.Second), want: map[string]any{"k": time.Second}},
		{name: "float64", field: zap.Float64("k", 1.5), want: map[string]any{"k": 1.5}},
		{name: "float32", field: zap.Float32("k", 1.5), want: map[string]any{"k": float32(1.5)}},
		{name: "int64", field: zap.Int64("k", -1), want: map[string]any{"k": int64(-1)}},
		{name: "int32", field: zap.Int32("k", -1), want: map[string]any{"k": int32(-1)}},
		{name: "int16", field: zap.Int16("k", -1), want: map[string]any{"k": int16(-1)}},
		{name: "int8", field: zap.Int8("k", -1), want: map[string]any{"k": int8(-1)}},
		{name: "time", field: zap.Time("k", now.In(time.UTC)), want: map[string]any{"k": now.In(time.UTC)}},
		{name: "time without location", field: Field{Key: "k", Type: zapcore.TimeType, Integer: now.UnixNano()}, want: map[string]any{"k": time.Unix(0, now.UnixNano())}},
		{name: "time full", field: zap.Time("k", time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)), want: map[string]any{"k": time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)}},
		{name: "uint64", field: zap.Uint64("k", 1), want: map[string]any{"k": uint64(1)}},
		{name: "uint32", field: zap.Uint32("k", 1), want: map[string]any{"k": uint32(1)}},
		{name: "uint16", field: zap.Uint16("k", 1), want: map[string]any{"k": uint16(1)}},
		{name: "uint8", field: zap.Uint8("k", 1), want: map[string]any{"k": uint8(1)}},
		{name: "uintptr", field: zap.Uintptr("k", 1), want: map[string]any{"k": uintptr(1)}},
		{name: "reflect", field: zap.Reflect("k", []int{1}), want: map[string]any{"k": []int{1}}},
		{name: "namespace", field: zap.Namespace("k"), want: map[string]any{"k": map[string]any{}}},
		{name: "stringer", field: zap.Stringer("k", stringer("v")), want: map[string]any{"k": "v"}},
		{name: "nil stringer", field: zap.Stringer("k", (*panicStringer)(nil)), want: map[string]any{"k": "<nil>"}},
		{name: "panicking stringer", field: zap.Stringer("k", &panicStringer{}), want: map[string]any{}, wantErr: true},
		{name: "error", field: zap.NamedError("k", errors.New("boom")), want: map[string]any{"k": "boom"}},
		{name: "skip", field: zap.Skip(), want: map[string]any{}},
		{name: "unknown type", field: Field{Key: "k", Type: zapcore.UnknownType}, want: map[string]any{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atts := newAttributes()
			atts.Add(tt.field)

			enc := zapcore.NewMapObjectEncoder()
			err := atts.MarshalLogObject(enc)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, enc.Fields)
		})
	}
}

func TestAttributesAdd(t *testing.T) {
	atts := newAttributes()
	atts.Add(zap.String("a", "1"))
	atts.Add(zap.String("b", "2"))
	atts.Add(zap.String("a", "3"))

	enc := zapcore.NewMapObjectEncoder()
	require.NoError(t, atts.MarshalLogObject(enc))
	assert.Equal(t, map[string]any{"a": "3", "b": "2"}, enc.Fields)
}

func TestAttributeField(t *testing.T) {
	atts := newAttributes()
	atts.Add(zap.String("k", "v"))

	f := attributeField(atts)
	assert.Equal(t, "Attributes", f.Key)
	assert.Equal(t, zapcore.ObjectMarshalerType, f.Type)

	enc := zapcore.NewMapObjectEncoder()
	f.AddTo(enc)
	assert.Equal(t, map[string]any{"Attributes": map[string]any{"k": "v"}}, enc.Fields)
}

func TestAttributeFieldsCaller(t *testing.T) {
	// attributeFields is called through Logger.<Level> -> log, emulate the same depth.
	var atts *attributes
	logAt := func() { atts = func() *attributes { return attributeFields(String("k", "v")) }() }
	logAt()

	enc := zapcore.NewMapObjectEncoder()
	require.NoError(t, atts.MarshalLogObject(enc))
	assert.Equal(t, "v", enc.Fields["k"])
	assert.Contains(t, fmt.Sprint(enc.Fields["caller.full_path"]), "attribute_test.go")
}
