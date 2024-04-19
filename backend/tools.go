//go:build tools
// +build tools

package tools

// enforce the version and listing in go.mod
// this is only required for proto code generation.
import (
	_ "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway"
	_ "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
