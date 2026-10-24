package minio

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/url"
	"time"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	miniogo "github.com/minio/minio-go/v7"
)

// 5MB is the absolute minimum part size for multipart uploads.
// MinIO throws error if the configured part size is less than this threshold.
// In the MinIO sdk, the default minimum for multipart uploads is 16MB.
const (
	minPartSize     = 5 * 1024 * 1024
	defaultPartSize = 16 * 1024 * 1024
)

var _ blob.Blob = (*Client)(nil)

// Client is a blob.Blob backed by a MinIO (or any S3 compatible) storage.
type Client struct {
	m *miniogo.Client
}

// New returns a Client using the given MinIO client.
func New(c *miniogo.Client) *Client {
	return &Client{
		m: c,
	}
}

// Exist implements blob.Blob.
func (c *Client) Exist(ctx context.Context, bucket, filename string) error {
	_, err := c.m.StatObject(ctx, bucket, filename, miniogo.StatObjectOptions{})
	if err != nil {
		return toBlobErr(err)
	}
	return nil
}

// Head implements blob.Blob.
func (c *Client) Head(ctx context.Context, bucket string, filename string) (*blob.ObjectMeta, error) {
	stat, err := c.m.StatObject(ctx, bucket, filename, miniogo.StatObjectOptions{})
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

// Reader implements blob.Blob.
func (c *Client) Reader(ctx context.Context, bucket string, filename string) (io.ReadCloser, error) {
	object, err := c.m.GetObject(ctx, bucket, filename, miniogo.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(blob.ErrInternal, err.Error())
	}
	r := &read{object}
	return r, nil
}

// Read implements blob.Blob.
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

// Write implements blob.Blob.
func (c *Client) Write(ctx context.Context, bucket string, filename string, data []byte, opts ...blob.WriteOption) error {
	putOpts := toPutOptions(opts...)
	if _, err := c.m.PutObject(ctx, bucket, filename, bytes.NewReader(data), int64(len(data)), *putOpts); err != nil {
		return toBlobErr(err)
	}
	return nil
}

// Writer implements blob.Blob.
// The upload runs in the background while the data is written, Close waits for it to complete.
func (c *Client) Writer(ctx context.Context, bucket string, filename string, opts ...blob.WriteOption) (io.WriteCloser, error) {
	putOpts := toStreamPutOptions(opts...)
	w := newPipe()
	go func() {
		// -1 as the size since it is unknown when streaming.
		_, err := c.m.PutObject(ctx, bucket, filename, w.pr, -1, *putOpts)
		w.finish(err)
	}()
	return w, nil
}

// Delete implements blob.Blob.
func (c *Client) Delete(ctx context.Context, bucket string, filename string) error {
	// to produce an error when the file is not exist in the bucket
	if err := c.Exist(ctx, bucket, filename); err != nil {
		return err
	}
	if err := c.m.RemoveObject(ctx, bucket, filename, miniogo.RemoveObjectOptions{}); err != nil {
		return toBlobErr(err)
	}
	return nil
}

// GetPresignedURL implements blob.Blob.
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

// PutPresignedURL implements blob.Blob.
func (c *Client) PutPresignedURL(ctx context.Context, bucket, filename string, expiry time.Duration) (*url.URL, error) {
	presignedURL, err := c.m.PresignedPutObject(ctx, bucket, filename, expiry)
	if err != nil {
		return nil, toBlobErr(err)
	}
	return presignedURL, nil
}

func toStreamPutOptions(options ...blob.WriteOption) *miniogo.PutObjectOptions {
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
	return &miniogo.PutObjectOptions{
		UserMetadata: opts.UserMetadata,
		UserTags:     opts.UserTags,
		ContentType:  opts.ContentType,
		PartSize:     opts.PartSize,
	}
}

func toPutOptions(options ...blob.WriteOption) *miniogo.PutObjectOptions {
	opts := &blob.WriteOptions{
		UserMetadata: make(map[string]string),
		UserTags:     make(map[string]string),
		ContentType:  "application/octet-stream",
		PartSize:     0,
	}
	for _, o := range options {
		o(opts)
	}

	return &miniogo.PutObjectOptions{
		UserMetadata: opts.UserMetadata,
		UserTags:     opts.UserTags,
		ContentType:  opts.ContentType,
		PartSize:     opts.PartSize,
	}
}
