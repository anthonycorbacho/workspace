package blobminio

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/url"
	"time"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	minio "github.com/minio/minio-go/v7"
)

// 5MB is the absolute minimum part size for multipart uploads.
// MinIO throws error if the configured part size is less than this threshold.
// In the MinIO sdk, the default minimum for multipart uploads is 16MB.
const (
	minPartSize     = 5 * 1024 * 1024
	defaultPartSize = 16 * 1024 * 1024
)

var _ blob.Blob = (*Client)(nil)

type Client struct {
	m *minio.Client
}

func New(c *minio.Client) *Client {
	return &Client{
		m: c,
	}
}

func (c *Client) Exist(ctx context.Context, bucket, filename string) error {
	_, err := c.m.StatObject(ctx, bucket, filename, minio.StatObjectOptions{})
	if err != nil {
		return toBlobErr(err)
	}
	return nil
}

func (c *Client) Head(ctx context.Context, bucket string, filename string) (*blob.ObjectMeta, error) {
	stat, err := c.m.StatObject(ctx, bucket, filename, minio.StatObjectOptions{})
	if err != nil {
		return nil, toBlobErr(err)
	}
	return &blob.ObjectMeta{
		Bucket:       bucket,
		Key:          filename,
		ETag:         stat.ETag,
		Size:         stat.Size,
		ContentType:  stat.ContentType,
		LastModified: stat.LastModified,
	}, nil
}

func (c *Client) Reader(ctx context.Context, bucket string, filename string) (io.ReadCloser, error) {
	object, err := c.m.GetObject(ctx, bucket, filename, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(blob.ErrInternal, err.Error())
	}
	r := &read{object}
	return r, nil
}

func (c *Client) Read(ctx context.Context, bucket string, filename string) ([]byte, error) {
	reader, err := c.Reader(ctx, bucket, filename)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(reader)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fileSize is opt if send -1 it will consider as streaming
func (c *Client) writer(
	ctx context.Context, bucket string, filename string, fileSize int64, putOpts *minio.PutObjectOptions, done chan<- error,
) *pipe {
	w := newPipe()
	go func() {
		_, err := c.m.PutObject(ctx, bucket, filename, w.pr, fileSize, *putOpts)
		w.pr.CloseWithError(err)
		if done != nil {
			done <- err
			close(done)
		}
	}()
	return w
}

// caller should close the returned writer before waiting on done
func (c *Client) Writer(ctx context.Context, bucket string, filename string, opts ...blob.WriteOption) (io.WriteCloser, error) {
	putOpts := toStreamPutOptions(opts...)
	w := c.writer(ctx, bucket, filename, -1, putOpts, nil)
	return w, nil
}

func (c *Client) Write(ctx context.Context, bucket string, filename string, data []byte, opts ...blob.WriteOption) error {
	putOpts := toPutOptions(opts...)
	w := c.writer(ctx, bucket, filename, int64(len(data)), putOpts, nil)
	_, err := w.Write(data)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) SyncWriter(ctx context.Context, bucket string, filename string, done chan<- error, opts ...blob.WriteOption) (io.WriteCloser, error) {
	putOpts := toStreamPutOptions(opts...)
	w := c.writer(ctx, bucket, filename, -1, putOpts, done)
	return w, nil
}

func (c *Client) SyncWrite(ctx context.Context, bucket string, filename string, data []byte, opts ...blob.WriteOption) error {
	putOpts := toPutOptions(opts...)
	if _, err := c.m.PutObject(ctx, bucket, filename, bytes.NewReader(data), int64(len(data)), *putOpts); err != nil {
		return toBlobErr(err)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, bucket string, filename string) error {
	// to produce an error when the file is not exist in the bucket
	if err := c.Exist(ctx, bucket, filename); err != nil {
		return err
	}
	if err := c.m.RemoveObject(ctx, bucket, filename, minio.RemoveObjectOptions{}); err != nil {
		return toBlobErr(err)
	}
	return nil
}

func (c *Client) GetPresignedURL(ctx context.Context, bucket, filename, alias string, expiry time.Duration) (*url.URL, error) {
	reqParams := make(url.Values)
	if alias != "" {
		reqParams.Set(
			"response-content-disposition",
			mime.FormatMediaType("attachment", map[string]string{"filename": alias}))
	} else {
		reqParams.Set("response-content-disposition", "attachment")
	}
	presignedURL, err := c.m.PresignedGetObject(ctx, bucket, filename, expiry, reqParams)
	if err != nil {
		return nil, toBlobErr(err)
	}
	return presignedURL, nil
}

func (c *Client) PutPresignedURL(ctx context.Context, bucket, filename string, expiry time.Duration) (*url.URL, error) {
	presignedURL, err := c.m.PresignedPutObject(ctx, bucket, filename, expiry)
	if err != nil {
		return nil, toBlobErr(err)
	}
	return presignedURL, nil
}

func toStreamPutOptions(options ...blob.WriteOption) *minio.PutObjectOptions {
	opts := &blob.WriteOptions{
		UserMetadata: make(map[string]string),
		UserTags:     make(map[string]string),
		ContentType:  "application/octet-stream",
		PartSize:     defaultPartSize,
	}
	for _, o := range options {
		o(opts)
	}
	if opts.PartSize < minPartSize {
		opts.PartSize = minPartSize
	}
	return &minio.PutObjectOptions{
		UserMetadata: opts.UserMetadata,
		UserTags:     opts.UserTags,
		ContentType:  opts.ContentType,
		PartSize:     opts.PartSize,
	}
}

func toPutOptions(options ...blob.WriteOption) *minio.PutObjectOptions {
	opts := &blob.WriteOptions{
		UserMetadata: make(map[string]string),
		UserTags:     make(map[string]string),
		ContentType:  "application/octet-stream",
		PartSize:     0,
	}
	for _, o := range options {
		o(opts)
	}

	return &minio.PutObjectOptions{
		UserMetadata: opts.UserMetadata,
		UserTags:     opts.UserTags,
		ContentType:  opts.ContentType,
		PartSize:     opts.PartSize,
	}
}
