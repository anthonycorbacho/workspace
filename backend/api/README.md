# API
This folder contains the API definition of all service. It leverages protobuf and grpc-gateway.

## Installation
Make sure you have Buf installed and buf module updated (run: `buf mod update`).

You'll use the protoc-gen-go and protoc-gen-go-grpc plugins to generate code with `buf generate`, so you'll need to install them:

```shell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/envoyproxy/protoc-gen-validate@latest
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2
```

You also need to update your PATH so that buf can find the plugins:

```shell
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Validation
We are using `envoyproxy/protoc-gen-validate` to validate proto fields and messages and, it will create `Validate` methods on the generated types.

See [Constraint Rules](https://github.com/bufbuild/protoc-gen-validate#constraint-rules)

eg:

```protobuf
syntax = "proto3";

package examplepb;

import "validate/validate.proto";

message Person {
  uint64 id    = 1 [(validate.rules).uint64.gt    = 999];
  string email = 2 [(validate.rules).string.email = true];
  string name  = 3 [(validate.rules).string = {
    pattern:   "^[^[0-9]A-Za-z]+( [^[0-9]A-Za-z]+)*$",
    max_bytes: 256,
  }];

  Location home = 4 [(validate.rules).message.required = true];

  message Location {
    double lat = 1 [(validate.rules).double = { gte: -90,  lte: 90 }];
    double lng = 2 [(validate.rules).double = { gte: -180, lte: 180 }];
  }
}
```
