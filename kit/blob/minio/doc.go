// Package minio implements blob.Blob on top of MinIO, it works with any S3 compatible storage.
//
// Use Config.NewClient to build a client from the configuration (MinIO or AWS S3 credentials),
// or New to wrap an existing MinIO client:
//
//	cfg := minio.Config{Provider: "minio", Endpoint: "localhost:9000", CredentialSource: "env"}
//	store, err := cfg.NewClient()
//	if err != nil {
//		return err
//	}
//	err = store.Write(ctx, "bucket", blob.MakeFilePath(companyID, "report.csv"), data, blob.WithContentTypeCSV())
//
// Errors are converted to blob.ErrNotFound and blob.ErrInternal.
package minio
