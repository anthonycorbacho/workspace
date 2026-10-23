package blobminio

import (
	"io"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/minio/minio-go/v7"
)

var (
	_ io.WriteCloser = (*pipe)(nil)
	_ io.ReadCloser  = (*read)(nil)
)

// write is a small wrapper around io.WriteCloser to handle the errors of minio
// it is a bit more complicated because  minio does not return a io.Writer but take an io.Reader in parameters
// so we use an in memory pipe. See io.Pipe() for doc
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

// read is a small wrapper around io.WriteCloser to handle the errors of minio
type read struct {
	s *minio.Object
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

	mErr := minio.ToErrorResponse(err)
	switch mErr.Code {
	case "NoSuchKey":
		return errors.Wrapf(blob.ErrNotFound, "%s: %s", mErr.Message, mErr.Key)
	case "NoSuchBucket":
		return errors.Wrapf(blob.ErrNotFound, "%s: %s", mErr.Message, mErr.BucketName)
	}
	return errors.Wrapf(blob.ErrInternal, "%v", err)
}
