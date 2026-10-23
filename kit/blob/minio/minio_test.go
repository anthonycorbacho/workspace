package blobminio

import (
	"context"
	"fmt"
	"mime"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/minio/minio-go/v7"
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
	client *Client
}

func (s *MinioTestSuite) SetupSuite() {
	if os.Getenv("MINIO_TEST_URL") == "" {
		s.T().Skip("No testing minio setup via env variable MINIO_TEST_URL")
	}

	s.url = os.Getenv("MINIO_TEST_URL")
	s.ctx = context.TODO()
	s.bucket = fmt.Sprintf("my-test-bucket-%d", time.Now().Unix())
	// NOTE: need to setup credentials via mc admin when testing locally
	c, err := minio.New(s.url, &minio.Options{
		Creds: credentials.NewStaticV4("miniouser", "miniopwd", ""),
	})
	if ok := s.Nil(err); !ok {
		s.FailNow("failed to create minio clients")
	}

	if ok, _ := c.BucketExists(s.ctx, s.bucket); !ok {
		err = c.MakeBucket(s.ctx, s.bucket, minio.MakeBucketOptions{})
		if ok := s.Nil(err); !ok {
			s.FailNow("failed to create minio test bucket")
		}
	}

	// we create bucket for the test

	s.client = New(c)

	err = s.client.Write(s.ctx, s.bucket, "myfile.txt", []byte("super doc"))
	if ok := s.Nil(err); !ok {
		s.FailNow("failed to create minio test file")
	}
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
	meta, err := s.client.Head(context.TODO(), s.bucket, "myfile.txt")
	s.Nil(err, "should not return error for existing file")
	s.NotNil(meta, "should return metadata for existing file")
	s.Equal(s.bucket, meta.Bucket, "bucket name should match")
	s.Equal("myfile.txt", meta.Key, "file name should match")
	s.Equal("application/octet-stream", meta.ContentType, "content type should match")
	s.Equal(int64(9), meta.Size, "size should be 0 for empty file")
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

func (s *MinioTestSuite) TestMinio_SyncWrite() {
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
			"mynewfile3.txt",
			"content",
			fmt.Sprintf("http://%s/%s/mynewfile3.txt", s.url, s.bucket),
			nil,
		},
		{
			"should succeed to ovewrite file",
			s.bucket,
			"mynewfile3.txt",
			"content 2",
			fmt.Sprintf("http://%s/%s/mynewfile3.txt", s.url, s.bucket),
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
			err := s.client.SyncWrite(context.TODO(), tt.inBucket, tt.inFileName, []byte(tt.inData))
			s.ErrorIs(err, tt.expectedError)
		})
	}
}
