package blobminio

import (
	"io"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
)

func TestToBlobErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantIs  error
		wantMsg string
	}{
		{
			name:    "EOF is kept",
			err:     io.EOF,
			wantIs:  io.EOF,
			wantMsg: "EOF",
		},
		{
			name:    "closed pipe is kept",
			err:     io.ErrClosedPipe,
			wantIs:  io.ErrClosedPipe,
			wantMsg: "io: read/write on closed pipe",
		},
		{
			name:    "no such key",
			err:     minio.ErrorResponse{Code: "NoSuchKey", Message: "missing key", Key: "file.txt"},
			wantIs:  blob.ErrNotFound,
			wantMsg: "missing key: file.txt: not found",
		},
		{
			name:    "no such bucket",
			err:     minio.ErrorResponse{Code: "NoSuchBucket", Message: "missing bucket", BucketName: "bucket"},
			wantIs:  blob.ErrNotFound,
			wantMsg: "missing bucket: bucket: not found",
		},
		{
			name:   "other minio error",
			err:    minio.ErrorResponse{Code: "AccessDenied", Message: "access denied"},
			wantIs: blob.ErrInternal,
		},
		{
			name:   "generic error",
			err:    errors.New("boom"),
			wantIs: blob.ErrInternal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toBlobErr(tt.err)
			assert.ErrorIs(t, got, tt.wantIs)
			if tt.wantMsg != "" {
				assert.EqualError(t, got, tt.wantMsg)
			}
		})
	}
}

func TestToPutOptions(t *testing.T) {
	tests := []struct {
		name     string
		convert  func(...blob.WriteOption) *minio.PutObjectOptions
		opts     []blob.WriteOption
		wantType string
		wantPart uint64
		wantMeta map[string]string
		wantTags map[string]string
	}{
		{
			name:     "put defaults",
			convert:  toPutOptions,
			wantType: "application/octet-stream",
			wantPart: 0,
			wantMeta: map[string]string{},
			wantTags: map[string]string{},
		},
		{
			name:    "put with options",
			convert: toPutOptions,
			opts: []blob.WriteOption{
				blob.WithContentTypeCsv(),
				blob.WithPartSize(1),
				blob.WithUserMetadata(map[string]string{"a": "1"}),
				blob.WithUserTags(map[string]string{"b": "2"}),
			},
			wantType: "text/csv",
			wantPart: 1,
			wantMeta: map[string]string{"a": "1"},
			wantTags: map[string]string{"b": "2"},
		},
		{
			name:     "stream defaults",
			convert:  toStreamPutOptions,
			wantType: "application/octet-stream",
			wantPart: defaultPartSize,
			wantMeta: map[string]string{},
			wantTags: map[string]string{},
		},
		{
			name:     "stream part size below minimum",
			convert:  toStreamPutOptions,
			opts:     []blob.WriteOption{blob.WithPartSize(1024)},
			wantType: "application/octet-stream",
			wantPart: minPartSize,
			wantMeta: map[string]string{},
			wantTags: map[string]string{},
		},
		{
			name:     "stream part size above minimum",
			convert:  toStreamPutOptions,
			opts:     []blob.WriteOption{blob.WithPartSize(minPartSize + 1), blob.WithContentType("text/plain")},
			wantType: "text/plain",
			wantPart: minPartSize + 1,
			wantMeta: map[string]string{},
			wantTags: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.convert(tt.opts...)
			assert.Equal(t, tt.wantType, got.ContentType)
			assert.Equal(t, tt.wantPart, got.PartSize)
			assert.Equal(t, tt.wantMeta, got.UserMetadata)
			assert.Equal(t, tt.wantTags, got.UserTags)
		})
	}
}
