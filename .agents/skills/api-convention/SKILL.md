---
name: api-convention
description: Create and evolve protobuf APIs under api/ following workspace conventions (AIP-style, grpc-gateway, Protovalidate).
compatibility: opencode
version: 1
---

# Create or Update Protobuf APIs

Create and modify protobuf APIs under `api/` using Google API conventions (AIP-style), while preserving backwards compatibility with existing surfaces.

## When To Use
- Adding a new `*.proto` under `api/**`.
- Adding new RPCs, messages, or fields to existing protos.
- Adding or updating HTTP/REST mappings via `google.api.http`.
- Adding validation rules via Protovalidate.

## Repository Context
- Protos live under `api/<namespace>/<service>/v1/`.
- Buf config: `api/buf.yaml` and `api/buf.gen.yaml`.
- Protoc plugins (`protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-grpc-gateway`, `protoc-gen-openapiv2`) are pinned in `go.mod` via the `tool` directive and executed via `go tool`.
- Validation uses Protovalidate: `buf/validate/validate.proto` with `(buf.validate.field)` and `(buf.validate.message).cel`. Rules are enforced automatically by `kit/grpc` server and client interceptors.
- HTTP/REST mapping uses `google/api/annotations.proto` and `option (google.api.http)`.
- OpenAPI output uses `protoc-gen-openapiv2/options/annotations.proto`.

## Non-Negotiables
- Do not rename existing RPCs, messages, fields, paths, or enums just to conform.
- Do not change enum numeric values.
- When removing fields, reserve numbers and names to prevent reuse.

## Commands

Run from repo root:

```bash
make lint-proto
make proto-gen
```

Or from `api/`:

```bash
buf lint
buf generate
```

## Conventions

### File, Package, Versioning
- Proto file names: `lower_snake_case.proto`.
- Package names: follow the folder hierarchy (`<namespace>.<service>.v1`), matching existing neighbors such as `sample.sampleapp.v1`.
- Set `option go_package` consistent with the module path and folder, e.g.:
  `option go_package = "github.com/anthonycorbacho/workspace/api/<namespace>/<service>/v1;<service>v1";`

### Service + RPC Naming
Prefer AIP-style noun services with standard verbs:
- Service: `<Name>Service` or plural noun matching neighbors.
- RPCs:
  - `GetX` / `FetchX`
  - `ListXs`
  - `CreateX`
  - `UpdateX`
  - `DeleteX`
  - Batch: `BatchCreateXs`, `BatchUpdateXs`, `BatchDeleteXs` (if supported)
  - Custom: only when not CRUD; map to `:verb` in HTTP.
  - Responses should usually include the populated resource unless there is a reason to return a partial response.

### HTTP Mapping
Default patterns:
- Get: `GET /v1/{collection}/{id}`
- List: `GET /v1/{collection}`
- Create: `POST /v1/{collection}` with `body: "*"` or `body: "<resource_field>"`
- Update: `PATCH /v1/{collection}/{resource.id}` with `body: "<resource_field>"`
- Delete: `DELETE /v1/{collection}/{id}`
- Custom: `POST /v1/{collection}/{id}:action` or `POST /v1/{collection}:action`

Use `additional_bindings` only when you truly must expose multiple equivalent routes.

### Pagination (List)
- Request: `page_size`, `page_token`
- Response: `next_page_token`, plus repeated resource field (compatible with `kit/pagination`)

### Update Semantics
- Use `google.protobuf.FieldMask update_mask`.
- Only mutate fields listed in `update_mask`.
- Consider `allow_missing` when explicit upsert semantics are desired.

## Validation (Protovalidate)
- Prefer `(buf.validate.field)` for single-field constraints.
- Use `(buf.validate.message).cel` for cross-field rules.

Examples:

```proto
import "buf/validate/validate.proto";

string email = 1 [(buf.validate.field).string.email = true];

repeated Foo foos = 2 [(buf.validate.field).repeated = { max_items: 10000 }];

option (buf.validate.message).cel = {
  id: "require_x"
  message: "x must be set when y is true"
  expression: "(!this.y) || has(this.x)"
};
```

## Templates

### CRUD + HTTP

```proto
syntax = "proto3";

package example.catalog.v1;
option go_package = "github.com/anthonycorbacho/workspace/api/example/catalog/v1;catalogv1";

import "buf/validate/validate.proto";
import "google/api/annotations.proto";
import "google/protobuf/field_mask.proto";

service CatalogService {
  rpc GetProduct(GetProductRequest) returns (GetProductResponse) {
    option (google.api.http) = { get: "/v1/products/{id}" };
  }

  rpc ListProducts(ListProductsRequest) returns (ListProductsResponse) {
    option (google.api.http) = { get: "/v1/products" };
  }

  rpc CreateProduct(CreateProductRequest) returns (CreateProductResponse) {
    option (google.api.http) = {
      post: "/v1/products"
      body: "product"
    };
  }

  rpc UpdateProduct(UpdateProductRequest) returns (UpdateProductResponse) {
    option (google.api.http) = {
      patch: "/v1/products/{product.id}"
      body: "product"
    };
  }

  rpc DeleteProduct(DeleteProductRequest) returns (DeleteProductResponse) {
    option (google.api.http) = { delete: "/v1/products/{id}" };
  }
}

message GetProductRequest {
  string id = 1 [(buf.validate.field).string.min_len = 1];
}
message GetProductResponse {
  Product product = 1;
}

message ListProductsRequest {
  int32 page_size = 1;
  string page_token = 2;
}
message ListProductsResponse {
  repeated Product products = 1;
  string next_page_token = 2;
}

message CreateProductRequest {
  Product product = 1 [(buf.validate.field).required = true];
}
message CreateProductResponse {
  Product product = 1;
}

message Product {
  string id = 1;
  string name = 2 [(buf.validate.field).string = { min_len: 1, max_len: 128 }];
}

message UpdateProductRequest {
  Product product = 1 [(buf.validate.field).required = true];
  google.protobuf.FieldMask update_mask = 2;
  bool allow_missing = 3;
}
message UpdateProductResponse {
  Product product = 1;
}

message DeleteProductRequest {
  string id = 1 [(buf.validate.field).string.min_len = 1];
}
message DeleteProductResponse {}
```

### Reserve Removed Fields

```proto
message Example {
  // reserved removed field numbers and names
  reserved 4, 7;
  reserved "old_field", "previous_name";
}
```

## Final Checklist
- `make lint-proto` passes.
- `make proto-gen` has been run and generated files are up to date.
- New protos follow `lower_snake_case.proto` and consistent `go_package`.
- Pagination uses `page_size`, `page_token`, `next_page_token`.
- Updates use `PATCH` + `update_mask`.
- No breaking renames; reserved numbers and names added for removals.
