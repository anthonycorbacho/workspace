package blobminio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{
			name:    "invalid provider",
			config:  Config{Provider: "gcs", Endpoint: "localhost:9000", CredentialSource: "static"},
			wantErr: "invalid blob storage provider: gcs",
		},
		{
			name:    "minio invalid credential source",
			config:  Config{Provider: "minio", Endpoint: "localhost:9000", CredentialSource: "iam"},
			wantErr: "invalid credential source for minio: iam",
		},
		{
			name:    "s3 invalid credential source",
			config:  Config{Provider: "s3", Endpoint: "localhost:9000", CredentialSource: ""},
			wantErr: "invalid credential source for s3: ",
		},
		{
			name:    "invalid endpoint",
			config:  Config{Provider: "minio", Endpoint: "localhost:9000/path", CredentialSource: "static"},
			wantErr: "Endpoint url cannot have fully qualified paths.",
		},
		{
			name: "minio static credentials",
			config: Config{
				Provider:          "minio",
				Endpoint:          "localhost:9000",
				CredentialSource:  "static",
				StaticCredentials: StaticCredentials{AccessKeyID: "key", SecretAccessKey: "secret"},
			},
		},
		{
			name:   "minio env credentials",
			config: Config{Provider: "minio", Endpoint: "localhost:9000", CredentialSource: "env"},
		},
		{
			name:   "s3 iam credentials",
			config: Config{Provider: "s3", Endpoint: "s3.amazonaws.com", Region: "us-east-1", UseSSL: true, CredentialSource: "iam"},
		},
		{
			name:   "s3 env credentials",
			config: Config{Provider: "s3", Endpoint: "s3.amazonaws.com", CredentialSource: "env"},
		},
		{
			name:   "s3 static credentials",
			config: Config{Provider: "s3", Endpoint: "s3.amazonaws.com", CredentialSource: "static"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := tt.config.NewClient()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)
			assert.IsType(t, &Client{}, c)
		})
	}
}
