package blob

import (
	"context"
	"io"
	"net/url"
	"time"
)

// Blob defines how we persist and access Blob data.
type Blob interface {
	// Exist returns error if the file does not exist
	Exist(ctx context.Context, bucket string, filename string) error

	// Head returns the metadata of a file in a bucket
	Head(ctx context.Context, bucket string, filename string) (*ObjectMeta, error)

	// Read returns the data of a file in a bucket
	// Should be used only if everything can fit in memory
	Read(ctx context.Context, bucket string, filename string) ([]byte, error)

	// Write (or overwrite) some data.
	Write(ctx context.Context, bucket string, filename string, data []byte, writeOpts ...WriteOption) error

	// Write (or overwrite) some data.
	SyncWrite(ctx context.Context, bucket string, filename string, data []byte, writeOpts ...WriteOption) error

	// Reader returns an io.ReadCloser that can be used to get chunks of data.
	// Should be used when we want to stream bytes or when file is too big to fit in memory
	Reader(ctx context.Context, bucket string, filename string) (io.ReadCloser, error)

	// Writer returns an io.WriteCloser that can be used to write (or overwrite) chunks of data
	// Should be used when we want to stream bytes or when file is too big to fit in memory
	Writer(ctx context.Context, bucket string, filename string, writeOpts ...WriteOption) (io.WriteCloser, error)

	// SyncWriter returns an io.WriteCloser that can be used to write (or overwrite) chunks of data
	// Should be used when we want to stream bytes or when file is too big to fit in memory
	// The done channel is used to signal when the underlying put operation is complete or if an error occurs
	// Note that the writer itself is non-blocking and must be closed before waiting on done
	SyncWriter(ctx context.Context, bucket string, filename string, done chan<- error, writeOpts ...WriteOption) (io.WriteCloser, error)

	// Delete a file in a bucket
	Delete(ctx context.Context, bucket string, filename string) error

	// Generate a signed URL to get a file in a bucket
	GetPresignedURL(ctx context.Context, bucket string, filename string, alias string, expiry time.Duration) (*url.URL, error)

	// Generate a signed URL to put a file in a bucket
	PutPresignedURL(ctx context.Context, bucket string, filename string, expiry time.Duration) (*url.URL, error)
}
