package minio

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/id"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/suite"
)

func TestMinioTestSuite(t *testing.T) {
	suite.Run(t, new(MinioTestSuite))
}

type MinioTestSuite struct {
	suite.Suite
	ctx    context.Context
	url    string
	bucket string
	minio  *miniogo.Client
	client *Client
}

func (s *MinioTestSuite) SetupSuite() {
	if os.Getenv("MINIO_TEST_URL") == "" {
		s.T().Skip("No testing minio setup via env variable MINIO_TEST_URL")
	}

	s.url = os.Getenv("MINIO_TEST_URL")
	s.ctx = context.TODO()
	s.bucket = fmt.Sprintf("my-test-bucket-%s", id.New())
	// NOTE: need to setup credentials via mc admin when testing locally
	c, err := miniogo.New(s.url, &miniogo.Options{
		Creds: credentials.NewStaticV4("miniouser", "miniopwd", ""),
	})
	if ok := s.Nil(err); !ok {
		s.FailNow("failed to create minio clients")
	}

	if ok, _ := c.BucketExists(s.ctx, s.bucket); !ok {
		err = c.MakeBucket(s.ctx, s.bucket, miniogo.MakeBucketOptions{})
		if ok := s.Nil(err); !ok {
			s.FailNow("failed to create minio test bucket")
		}
	}

	// we create bucket for the test

	s.minio = c
	s.client = New(c)

	err = s.client.Write(s.ctx, s.bucket, "myfile.txt", []byte("super doc"))
	if ok := s.Nil(err); !ok {
		s.FailNow("failed to create minio test file")
	}
}

func (s *MinioTestSuite) TearDownSuite() {
	if s.minio == nil {
		return
	}
	for obj := range s.minio.ListObjects(s.ctx, s.bucket, miniogo.ListObjectsOptions{Recursive: true}) {
		if obj.Err == nil {
			_ = s.minio.RemoveObject(s.ctx, s.bucket, obj.Key, miniogo.RemoveObjectOptions{})
		}
	}
	_ = s.minio.RemoveBucket(s.ctx, s.bucket)
}

func (s *MinioTestSuite) SetupTest() {}

func (s *MinioTestSuite) AfterTest() {}

func (s *MinioTestSuite) TestMinio_Write() {
	type testStruct struct {
		name          string
		inBucket      string
		inFileName    string
		inData        string
		want          string
		expectedError error
	}
	tests := []testStruct{
		// might be an issue in the emulator (or default behavior)
		// it create the bucket by default
		{
			"should get error: bucket does not exist",
			"writebucket",
			"myfile.txt",
			"content",
			"",
			blob.ErrNotFound,
		},
		{
			"should succeed to write new file",
			s.bucket,
			"mynewfile.txt",
			"content",
			fmt.Sprintf("http://%s/%s/mynewfile.txt", s.url, s.bucket),
			nil,
		},
		{
			"should succeed to ovewrite file",
			s.bucket,
			"mynewfile.txt",
			"content 2",
			fmt.Sprintf("http://%s/%s/mynewfile.txt", s.url, s.bucket),
			nil,
		},
		{
			"should succeed to write empty file",
			s.bucket,
			"myemptyfile.txt",
			"",
			fmt.Sprintf("http://%s/%s/myemptyfile.txt", s.url, s.bucket),
			nil,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := s.client.Write(context.TODO(), tt.inBucket, tt.inFileName, []byte(tt.inData))
			s.ErrorIs(err, tt.expectedError)
		})
	}
}

func (s *MinioTestSuite) TestMinio_Writer() {
	type testStruct struct {
		name          string
		inBucket      string
		inFileName    string
		inData        []byte
		wantWritten   int
		expectedError error
	}
	tests := []testStruct{
		{
			"should succeed to write new file",
			s.bucket,
			"mynewfile2.txt",
			[]byte("content"),
			7,
			nil,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			writer, _ := s.client.Writer(context.TODO(), tt.inBucket, tt.inFileName)
			n, err := writer.Write(tt.inData)
			writer.Close()
			s.Equal(tt.expectedError, err, "error should be equal to expected error")
			if err == nil {
				s.Equal(tt.wantWritten, n, "data written to minio test file should be equal to data sent")
			}
		})
	}
}

func (s *MinioTestSuite) TestMinio_Read() {
	type testStruct struct {
		name          string
		inBucket      string
		inFileName    string
		want          string
		expectedError error
	}
	tests := []testStruct{
		{
			"should get error: bucket does not exist",
			"readbucket",
			"myfileaaaa.txt",
			"",
			errors.Wrap(blob.ErrNotFound, "The specified bucket does not exist: readbucket"),
		},
		{
			"should get error: file does not exist",
			s.bucket,
			"myfileaaaa.txt",
			"",
			errors.Wrap(blob.ErrNotFound, "The specified key does not exist.: myfileaaaa.txt"),
		},
		{
			"should succeed",
			s.bucket,
			"myfile.txt",
			"super doc",
			nil,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.client.Read(context.TODO(), tt.inBucket, tt.inFileName)
			s.Equal(tt.expectedError, err)
			s.Equal(tt.want, string(got))
		})
	}
}

func (s *MinioTestSuite) TestMinio_Exist() {
	type testStruct struct {
		name          string
		inBucket      string
		inFileName    string
		expectedError error
	}
	tests := []testStruct{
		{
			"should get error: bucket does not exist",
			"writebucket",
			"myfile.txt",
			errors.Wrap(blob.ErrNotFound, "The specified bucket does not exist: writebucket"),
		},
		{
			"should get error: file does not exist",
			s.bucket,
			"mynewfileyolo.txt",
			errors.Wrap(blob.ErrNotFound, "The specified key does not exist.: mynewfileyolo.txt"),
		},
		{
			"should succeed if file exist",
			s.bucket,
			"myfile.txt",
			nil,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := s.client.Exist(context.TODO(), tt.inBucket, tt.inFileName)
			s.Equal(tt.expectedError, err)
		})
	}
}

func (s *MinioTestSuite) TestMinio_Head() {
	err := s.client.Write(s.ctx, s.bucket, "head.csv", []byte("a,b\n1,2\n"), blob.WithContentTypeCSV())
	s.Require().NoError(err)

	tests := []struct {
		name          string
		inBucket      string
		inFileName    string
		wantType      string
		wantSize      int64
		expectedError error
	}{
		{
			name:       "default content type",
			inBucket:   s.bucket,
			inFileName: "myfile.txt",
			wantType:   "application/octet-stream",
			wantSize:   9,
		},
		{
			name:       "custom content type",
			inBucket:   s.bucket,
			inFileName: "head.csv",
			wantType:   "text/csv",
			wantSize:   8,
		},
		{
			name:          "file does not exist",
			inBucket:      s.bucket,
			inFileName:    "missing.txt",
			expectedError: blob.ErrNotFound,
		},
		{
			name:          "bucket does not exist",
			inBucket:      "headbucket",
			inFileName:    "myfile.txt",
			expectedError: blob.ErrNotFound,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			meta, err := s.client.Head(s.ctx, tt.inBucket, tt.inFileName)
			if tt.expectedError != nil {
				s.ErrorIs(err, tt.expectedError)
				s.Nil(meta)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.inBucket, meta.Bucket)
			s.Equal(tt.inFileName, meta.Key)
			s.Equal(tt.wantType, meta.ContentType)
			s.Equal(tt.wantSize, meta.Size)
			s.NotEmpty(meta.ETag)
			s.WithinDuration(time.Now(), meta.LastModified, time.Hour)
		})
	}
}

func (s *MinioTestSuite) TestMinio_WriteOptions() {
	tests := []struct {
		name         string
		write        func(filename string, opts ...blob.WriteOption) error
		opts         []blob.WriteOption
		wantType     string
		wantMetadata map[string]string
		wantTags     map[string]string
	}{
		{
			name: "write with options",
			write: func(filename string, opts ...blob.WriteOption) error {
				return s.client.Write(s.ctx, s.bucket, filename, []byte("data"), opts...)
			},
			opts: []blob.WriteOption{
				blob.WithContentType("application/json"),
				blob.WithUserMetadata(map[string]string{"Owner": "kit"}),
				blob.WithUserTags(map[string]string{"env": "test"}),
			},
			wantType:     "application/json",
			wantMetadata: map[string]string{"Owner": "kit"},
			wantTags:     map[string]string{"env": "test"},
		},
		{
			name: "writer with options",
			write: func(filename string, opts ...blob.WriteOption) error {
				w, err := s.client.Writer(s.ctx, s.bucket, filename, opts...)
				if err != nil {
					return err
				}
				if _, err := w.Write([]byte("data")); err != nil {
					return err
				}
				return w.Close()
			},
			opts: []blob.WriteOption{
				blob.WithContentType("text/plain"),
				blob.WithPartSize(1),
				blob.WithUserTags(map[string]string{"env": "test"}),
			},
			wantType:     "text/plain",
			wantMetadata: map[string]string{},
			wantTags:     map[string]string{"env": "test"},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			filename := id.New() + ".txt"
			s.Require().NoError(tt.write(filename, tt.opts...))

			stat, err := s.minio.StatObject(s.ctx, s.bucket, filename, miniogo.StatObjectOptions{})
			s.Require().NoError(err)
			s.Equal(tt.wantType, stat.ContentType)
			for k, v := range tt.wantMetadata {
				s.Equal(v, stat.UserMetadata[k], k)
			}

			tags, err := s.minio.GetObjectTagging(s.ctx, s.bucket, filename, miniogo.GetObjectTaggingOptions{})
			s.Require().NoError(err)
			s.Equal(tt.wantTags, tags.ToMap())

			data, err := s.client.Read(s.ctx, s.bucket, filename)
			s.Require().NoError(err)
			s.Equal("data", string(data))
		})
	}
}

func (s *MinioTestSuite) TestMinio_Reader() {
	tests := []struct {
		name          string
		inBucket      string
		inFileName    string
		want          string
		expectedError error
	}{
		{name: "existing file", inBucket: s.bucket, inFileName: "myfile.txt", want: "super doc"},
		{name: "file does not exist", inBucket: s.bucket, inFileName: "missing.txt", expectedError: blob.ErrNotFound},
		{name: "bucket does not exist", inBucket: "readerbucket", inFileName: "myfile.txt", expectedError: blob.ErrNotFound},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			r, err := s.client.Reader(s.ctx, tt.inBucket, tt.inFileName)
			// the object is fetched lazily, errors are returned on read.
			s.Require().NoError(err)
			defer func() { s.NoError(r.Close()) }()

			got, err := io.ReadAll(r)
			s.ErrorIs(err, tt.expectedError)
			s.Equal(tt.want, string(got))
		})
	}
}

func (s *MinioTestSuite) TestMinio_ReadCanceledContext() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	_, err := s.client.Read(ctx, s.bucket, "myfile.txt")
	s.ErrorIs(err, blob.ErrInternal)
}

func (s *MinioTestSuite) TestMinio_PresignedURLs() {
	filename := id.New() + ".txt"

	// upload through the presigned PUT URL.
	putURL, err := s.client.PutPresignedURL(s.ctx, s.bucket, filename, time.Minute)
	s.Require().NoError(err)
	req, err := http.NewRequestWithContext(s.ctx, http.MethodPut, putURL.String(), strings.NewReader("presigned content"))
	s.Require().NoError(err)
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	_ = resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	tests := []struct {
		name            string
		alias           string
		wantDisposition string
	}{
		{name: "without alias", alias: "", wantDisposition: "attachment"},
		{name: "with alias", alias: "report.txt", wantDisposition: `attachment; filename=report.txt`},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			getURL, err := s.client.GetPresignedURL(s.ctx, s.bucket, filename, tt.alias, time.Minute)
			s.Require().NoError(err)

			req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, getURL.String(), nil)
			s.Require().NoError(err)
			resp, err := http.DefaultClient.Do(req)
			s.Require().NoError(err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Equal(http.StatusOK, resp.StatusCode)
			s.Equal("presigned content", string(body))
			s.Equal(tt.wantDisposition, resp.Header.Get("Content-Disposition"))
		})
	}
}

func (s *MinioTestSuite) TestMinio_Delete() {
	type testStruct struct {
		name          string
		inBucket      string
		inFileName    string
		expectedError error
	}
	err := s.client.Write(s.ctx, s.bucket, "filetobedeleted.txt", []byte("file to be deleted"))
	if ok := s.Nil(err); !ok {
		s.FailNow("failed to create minio test file")
	}
	tests := []testStruct{
		{
			"should get error: bucket does not exist",
			"writebucket",
			"filetobedeleted.txt",
			errors.Wrap(blob.ErrNotFound, "The specified bucket does not exist: writebucket"),
		},
		{
			"should get error: file does not exist",
			s.bucket,
			"mynewfileyolo.txt",
			errors.Wrap(blob.ErrNotFound, "The specified key does not exist.: mynewfileyolo.txt"),
		},
		{
			"should succeed if file exist",
			s.bucket,
			"filetobedeleted.txt",
			nil,
		},
		{
			"should get error: file does not exist",
			s.bucket,
			"filetobedeleted.txt",
			errors.Wrap(blob.ErrNotFound, "The specified key does not exist.: filetobedeleted.txt"),
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := s.client.Delete(context.TODO(), tt.inBucket, tt.inFileName)
			s.Equal(tt.expectedError, err)
		})
	}
}

func (s *MinioTestSuite) TestMinio_GetPresignedURL() {
	type testStruct struct {
		name          string
		inBucket      string
		inFileName    string
		inAlias       string
		inExpiry      time.Duration
		want          string
		expectedError error
	}
	tests := []testStruct{
		{
			"should get error: bucket does not exist",
			"writebucket",
			"myfile.txt",
			"",
			time.Hour,
			"",
			errors.Wrap(blob.ErrNotFound, "The specified bucket does not exist: writebucket"),
		},
		{
			"should succeed",
			s.bucket,
			"myfile.txt",
			"testalias.txt",
			time.Hour,
			fmt.Sprintf("http://localhost:9000/%s/myfile.txt", s.bucket),
			nil,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.client.GetPresignedURL(context.TODO(), tt.inBucket, tt.inFileName, tt.inAlias, tt.inExpiry)
			s.Equal(tt.expectedError, err)
			if tt.expectedError == nil {
				s.Equal(tt.want, strings.Split(got.String(), "?")[0])
				wantProp := "attachment"
				if len(tt.inAlias) > 0 {
					wantProp = mime.FormatMediaType("attachment", map[string]string{"filename": tt.inAlias})
				}
				prop := got.Query().Get("response-content-disposition")
				s.Equal(wantProp, prop)
			}
		})
	}
}

func (s *MinioTestSuite) TestMinio_PutPresignedURL() {
	type testStruct struct {
		name          string
		inBucket      string
		inFileName    string
		inExpiry      time.Duration
		want          string
		expectedError error
	}
	tests := []testStruct{
		{
			"should get error: bucket does not exist",
			"writebucket",
			"myfile.txt",
			time.Hour,
			"",
			errors.Wrap(blob.ErrNotFound, "The specified bucket does not exist: writebucket"),
		},
		{
			"should succeed",
			s.bucket,
			"want_to_put.txt",
			time.Hour,
			fmt.Sprintf("http://localhost:9000/%s/want_to_put.txt", s.bucket),
			nil,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.client.PutPresignedURL(context.TODO(), tt.inBucket, tt.inFileName, tt.inExpiry)
			s.Equal(tt.expectedError, err)
			if tt.expectedError == nil {
				s.Equal(tt.want, strings.Split(got.String(), "?")[0])
			}
		})
	}
}

func (s *MinioTestSuite) TestMinio_WriterCloseWaitsForUpload() {
	w, err := s.client.Writer(context.TODO(), s.bucket, "streamed.txt")
	s.Require().NoError(err)
	_, err = w.Write([]byte("streamed content"))
	s.Require().NoError(err)
	s.Require().NoError(w.Close())

	// the upload is completed once Close returned.
	data, err := s.client.Read(context.TODO(), s.bucket, "streamed.txt")
	s.Require().NoError(err)
	s.Equal("streamed content", string(data))
}

func (s *MinioTestSuite) TestMinio_WriterCloseReturnsUploadError() {
	w, err := s.client.Writer(context.TODO(), "missingbucket", "streamed.txt")
	s.Require().NoError(err)
	_, err = w.Write([]byte("streamed content"))
	if err == nil {
		err = w.Close()
	}
	s.ErrorIs(err, blob.ErrNotFound)
}
