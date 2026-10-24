package blob_test

import (
	"testing"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/stretchr/testify/assert"
)

func TestWriteOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []blob.WriteOption
		want blob.WriteOptions
	}{
		{
			name: "no options",
			want: blob.WriteOptions{},
		},
		{
			name: "content type",
			opts: []blob.WriteOption{blob.WithContentType("application/json")},
			want: blob.WriteOptions{ContentType: "application/json"},
		},
		{
			name: "content type csv",
			opts: []blob.WriteOption{blob.WithContentTypeCSV()},
			want: blob.WriteOptions{ContentType: "text/csv"},
		},
		{
			name: "last content type wins",
			opts: []blob.WriteOption{blob.WithContentTypeCSV(), blob.WithContentType("text/plain")},
			want: blob.WriteOptions{ContentType: "text/plain"},
		},
		{
			name: "part size",
			opts: []blob.WriteOption{blob.WithPartSize(42)},
			want: blob.WriteOptions{PartSize: 42},
		},
		{
			name: "user metadata are merged",
			opts: []blob.WriteOption{
				blob.WithUserMetadata(map[string]string{"a": "1", "b": "2"}),
				blob.WithUserMetadata(map[string]string{"b": "3", "c": "4"}),
			},
			want: blob.WriteOptions{UserMetadata: map[string]string{"a": "1", "b": "3", "c": "4"}},
		},
		{
			name: "user tags are merged",
			opts: []blob.WriteOption{
				blob.WithUserTags(map[string]string{"a": "1", "b": "2"}),
				blob.WithUserTags(map[string]string{"b": "3"}),
			},
			want: blob.WriteOptions{UserTags: map[string]string{"a": "1", "b": "3"}},
		},
		{
			name: "empty user metadata and tags",
			opts: []blob.WriteOption{blob.WithUserMetadata(nil), blob.WithUserTags(nil)},
			want: blob.WriteOptions{UserMetadata: map[string]string{}, UserTags: map[string]string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got blob.WriteOptions
			for _, o := range tt.opts {
				o(&got)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWriteOptionsCopyMaps(t *testing.T) {
	tests := []struct {
		name   string
		option func(map[string]string) blob.WriteOption
		got    func(blob.WriteOptions) map[string]string
	}{
		{
			name:   "user metadata",
			option: blob.WithUserMetadata,
			got:    func(o blob.WriteOptions) map[string]string { return o.UserMetadata },
		},
		{
			name:   "user tags",
			option: blob.WithUserTags,
			got:    func(o blob.WriteOptions) map[string]string { return o.UserTags },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := map[string]string{"key": "value"}
			var opts blob.WriteOptions
			tt.option(src)(&opts)

			// mutating the given map must not change the options.
			src["key"] = "changed"
			src["other"] = "value"
			assert.Equal(t, map[string]string{"key": "value"}, tt.got(opts))
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "not found", err: blob.ErrNotFound, want: "not found"},
		{name: "internal", err: blob.ErrInternal, want: "internal error"},
		{name: "custom", err: blob.Error("custom"), want: "custom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.err, tt.want)
		})
	}
}
