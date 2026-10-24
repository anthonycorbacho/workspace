package minio

import (
	"io"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	miniogo "github.com/minio/minio-go/v7"
)

var (
	_ io.WriteCloser = (*pipe)(nil)
	_ io.ReadCloser  = (*read)(nil)
)

// pipe is an io.WriteCloser feeding a MinIO upload.
// MinIO does not return an io.Writer but takes an io.Reader, so the written data goes through an in memory pipe.
// See io.Pipe for details.
type pipe struct {
	pr *io.PipeReader
	pw *io.PipeWriter
	// finished is closed once the upload reading from the pipe completed, err holds its result.
	finished chan struct{}
	err      error
}

func newPipe() *pipe {
	pr, pw := io.Pipe()
	return &pipe{pr: pr, pw: pw, finished: make(chan struct{})}
}

// finish records the upload result and unblocks Close.
func (w *pipe) finish(err error) {
	w.err = err
	w.pr.CloseWithError(err)
	close(w.finished)
}

// read is a small wrapper around a MinIO object to convert its errors to blob errors.
type read struct {
	s *miniogo.Object
}

// Write implements io.WriteCloser
func (w *pipe) Write(p []byte) (int, error) {
	n, err := w.pw.Write(p)
	if err != nil {
		return n, toBlobErr(err)
	}
	return n, nil
}

// Close implements io.WriteCloser.
// It waits for the upload to complete so the upload error is not lost.
func (w *pipe) Close() error {
	err := w.pw.Close()
	if err != nil {
		return errors.Wrap(blob.ErrInternal, err.Error())
	}
	<-w.finished
	if w.err != nil {
		return toBlobErr(w.err)
	}
	return nil
}

// Read implements io.ReadCloser
func (r *read) Read(p []byte) (int, error) {
	n, err := r.s.Read(p)
	if err != nil {
		return n, toBlobErr(err)
	}
	return n, nil
}

// Close implements io.ReadCloser
func (r *read) Close() error {
	err := r.s.Close()
	if err != nil {
		return errors.Wrap(blob.ErrInternal, err.Error())
	}
	return nil
}

func toBlobErr(err error) error {
	if errors.Is(err, io.EOF) {
		return err
	}
	if errors.Is(err, io.ErrClosedPipe) {
		return err
	}

	mErr := miniogo.ToErrorResponse(err)
	switch mErr.Code {
	case "NoSuchKey":
		return errors.Wrapf(blob.ErrNotFound, "%s: %s", mErr.Message, mErr.Key)
	case "NoSuchBucket":
		return errors.Wrapf(blob.ErrNotFound, "%s: %s", mErr.Message, mErr.BucketName)
	}
	return errors.Wrapf(blob.ErrInternal, "%v", err)
}
