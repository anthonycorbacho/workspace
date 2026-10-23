package blobminio

import (
	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config represents a minio blob storage configuration
type Config struct {
	Provider          string `yaml:"provider" env:"BLOB_PROVIDER,overwrite,default=minio"` // minio, s3
	Endpoint          string `yaml:"endpoint" env:"BLOB_ENDPOINT,overwrite,default="`
	Region            string `yaml:"region" env:"BLOB_REGION,overwrite,default="`
	UseSSL            bool   `yaml:"useSSL" env:"BLOB_USE_SSL,overwrite,default=false"`
	CredentialSource  string `yaml:"credentialSource" env:"BLOB_CREDENTIAL_SOURCE,overwrite,default="` // iam, env, static
	StaticCredentials `yaml:",inline"`
}

type StaticCredentials struct {
	AccessKeyID     string `yaml:"accessKeyId" env:"BLOB_ACCESS_KEY_ID,overwrite,default="`
	SecretAccessKey string `yaml:"secretAccessKey" env:"BLOB_SECRET_ACCESS_KEY,overwrite,default="`
}

// NewClient creates a new blob storage client based on the given configuration
func (c *Config) NewClient() (blob.Blob, error) {
	switch c.Provider {
	case "minio":
		return c.newMinioClient()
	case "s3":
		return c.newS3Client()
	default:
		return nil, errors.Newf("invalid blob storage provider: %s", c.Provider)
	}
}

func (c *Config) newMinioClient() (blob.Blob, error) {
	var creds *credentials.Credentials
	switch c.CredentialSource {
	case "env":
		creds = credentials.NewEnvMinio()
	case "static":
		creds = credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, "")
	default:
		return nil, errors.Newf("invalid credential source for minio: %s", c.CredentialSource)
	}
	return c.newClient(creds)
}

func (c *Config) newS3Client() (blob.Blob, error) {
	var creds *credentials.Credentials
	switch c.CredentialSource {
	case "iam":
		creds = credentials.NewIAM("")
	case "env":
		creds = credentials.NewEnvAWS()
	case "static":
		creds = credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, "")
	default:
		return nil, errors.Newf("invalid credential source for s3: %s", c.CredentialSource)
	}
	return c.newClient(creds)
}

func (c *Config) newClient(creds *credentials.Credentials) (blob.Blob, error) {
	cli, err := minio.New(c.Endpoint, &minio.Options{
		Creds:  creds,
		Secure: c.UseSSL,
		Region: c.Region,
	})
	if err != nil {
		return nil, err
	}
	return New(cli), nil
}
