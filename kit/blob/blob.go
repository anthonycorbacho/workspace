package blob

import (
	"context"
	"io"
	"net/url"
	"time"
)

// Blob defines how we persist and access Blob data.
type Blob interface {
	// Exist returns blob.ErrNotFound if the file or the bucket does not exist.
	Exist(ctx context.Context, bucket string, filename string) error

	// Head returns the metadata of a file in a bucket without reading its content.
	Head(ctx context.Context, bucket string, filename string) (*ObjectMeta, error)

	// Read returns the data of a file in a bucket.
	// Should be used only if everything can fit in memory.
	Read(ctx context.Context, bucket string, filename string) ([]byte, error)

	// Write writes (or overwrites) a file in a bucket, it returns once the upload completed.
	Write(ctx context.Context, bucket string, filename string, data []byte, writeOpts ...WriteOption) error

	// Reader returns an io.ReadCloser that can be used to get chunks of data.
	// Should be used when we want to stream bytes or when file is too big to fit in memory.
	Reader(ctx context.Context, bucket string, filename string) (io.ReadCloser, error)

	// Writer returns an io.WriteCloser that can be used to write (or overwrite) chunks of data.
	// Should be used when we want to stream bytes or when file is too big to fit in memory.
	// Close waits for the upload to complete and returns its error, so it must always be checked.
	Writer(ctx context.Context, bucket string, filename string, writeOpts ...WriteOption) (io.WriteCloser, error)

	// Delete removes a file from a bucket, it returns blob.ErrNotFound if the file does not exist.
	Delete(ctx context.Context, bucket string, filename string) error

	// GetPresignedURL generates a signed URL to download a file from a bucket.
	// The file is served as an attachment, named alias when it is not empty.
	GetPresignedURL(ctx context.Context, bucket string, filename string, alias string, expiry time.Duration) (*url.URL, error)

	// PutPresignedURL generates a signed URL to upload a file to a bucket.
	PutPresignedURL(ctx context.Context, bucket string, filename string, expiry time.Duration) (*url.URL, error)
}
