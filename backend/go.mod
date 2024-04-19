module github.com/anthonycorbacho/workspace

go 1.20

require (
	cloud.google.com/go/pubsub v1.32.0
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/envoyproxy/protoc-gen-validate v1.0.2
	github.com/golang-migrate/migrate/v4 v4.16.2
	github.com/golang/protobuf v1.5.3
	github.com/gorilla/handlers v1.4.2
	github.com/gorilla/mux v1.8.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.0
	github.com/jackc/pgx/v4 v4.18.1
	github.com/jmoiron/sqlx v1.3.5
	github.com/nats-io/nats.go v1.27.1
	github.com/prometheus/client_golang v1.16.0
	github.com/prometheus/common v0.44.0
	github.com/redis/go-redis/extra/redisotel/v9 v9.0.5
	github.com/redis/go-redis/v9 v9.0.5
	github.com/rs/cors v1.9.0
	github.com/rs/xid v1.5.0
	github.com/sethvargo/go-envconfig v0.9.0
	github.com/slok/go-http-metrics v0.10.0
	github.com/stretchr/testify v1.8.4
	github.com/uptrace/opentelemetry-go-extra/otelsql v0.2.2
	github.com/uptrace/opentelemetry-go-extra/otelsqlx v0.2.2
	github.com/vmihailenco/msgpack/v5 v5.3.5
	github.com/xo/dburl v0.14.2
	go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux v0.42.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.42.0
	go.opentelemetry.io/contrib/propagators/b3 v1.17.0
	go.opentelemetry.io/otel v1.16.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.16.0
	go.opentelemetry.io/otel/exporters/prometheus v0.39.0
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/sdk/metric v0.39.0
	go.opentelemetry.io/otel/trace v1.16.0
	go.uber.org/automaxprocs v1.5.2
	go.uber.org/zap v1.24.0
	google.golang.org/genproto/googleapis/api v0.0.0-20230626202813-9b080da550b3
	google.golang.org/grpc v1.56.1
	google.golang.org/protobuf v1.31.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cloud.google.com/go v0.110.3 // indirect
	cloud.google.com/go/compute v1.20.1 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v1.1.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/glog v1.1.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/s2a-go v0.1.4 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.5 // indirect
	github.com/googleapis/gax-go/v2 v2.11.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.0 // indirect
	github.com/jackc/pgerrcode v0.0.0-20220416144525-469b46aa5efa // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.2 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgtype v1.14.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.16.6 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/nats-io/nats-server/v2 v2.9.15 // indirect
	github.com/nats-io/nkeys v0.4.4 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/procfs v0.11.0 // indirect
	github.com/redis/go-redis/extra/rediscmd/v9 v9.0.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.16.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.opentelemetry.io/proto/otlp v0.20.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.10.0 // indirect
	golang.org/x/net v0.11.0 // indirect
	golang.org/x/oauth2 v0.9.0 // indirect
	golang.org/x/sync v0.3.0 // indirect
	golang.org/x/sys v0.9.0 // indirect
	golang.org/x/text v0.10.0 // indirect
	google.golang.org/api v0.129.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230626202813-9b080da550b3 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230626202813-9b080da550b3 // indirect
)
