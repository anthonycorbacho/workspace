# API
This folder contains the API definition of all service. It leverages protobuf and grpc-gateway.

## Installation
Make sure you have [Buf](https://buf.build/docs/installation) installed.

The protoc plugins (protoc-gen-go, protoc-gen-go-grpc, protoc-gen-grpc-gateway and protoc-gen-openapiv2) are pinned in the
`go.mod` with the `tool` directive and run by buf with `go tool`, so there is nothing else to install.

```shell
# update the buf dependencies (buf.lock)
buf dep update
# generate the code (or run `make proto-gen` from the root of the workspace)
buf generate
```

To update the plugins, run `go get -tool <plugin>@latest` from the root of the workspace.

## Validation
We are using [protovalidate](https://github.com/bufbuild/protovalidate) to validate proto fields and messages.
The validation rules are enforced by the kit gRPC server (and client) interceptors, so invalid requests are rejected with an `InvalidArgument` error.
A message can also be validated manually with `protovalidate.Validate(msg)`.

See [Standard rules](https://protovalidate.com/schemas/standard-rules/)

eg:

```protobuf
syntax = "proto3";

package examplepb;

import "buf/validate/validate.proto";

message Person {
  uint64 id    = 1 [(buf.validate.field).uint64.gt    = 999];
  string email = 2 [(buf.validate.field).string.email = true];
  string name  = 3 [(buf.validate.field).string = {
    pattern:   "^[^[0-9]A-Za-z]+( [^[0-9]A-Za-z]+)*$",
    max_bytes: 256,
  }];

  Location home = 4 [(buf.validate.field).required = true];

  message Location {
    double lat = 1 [(buf.validate.field).double = { gte: -90,  lte: 90 }];
    double lng = 2 [(buf.validate.field).double = { gte: -180, lte: 180 }];
  }
}
```
