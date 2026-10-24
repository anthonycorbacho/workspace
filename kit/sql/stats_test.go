package sql

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// maxOpenConnsPoints returns the number of databases reporting their max open connections.
func maxOpenConnsPoints(t *testing.T, reader *sdkmetric.ManualReader) int {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	n := 0
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "go.sql.connections_max_open" {
				n += len(m.Data.(metricdata.Gauge[int64]).DataPoints)
			}
		}
	}
	return n
}

// useMeterProvider reports the database metrics of the test to a manual reader.
func useMeterProvider(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	prev := meterProvider
	meterProvider = func() metric.MeterProvider { return mp }
	t.Cleanup(func() { meterProvider = prev })
	return reader
}

func TestCloseReleasesMetrics(t *testing.T) {
	tests := []struct {
		name       string
		open       int
		close      int
		wantPoints int
	}{
		{name: "opened databases are reported once", open: 3, close: 0, wantPoints: 3},
		{name: "closed databases are not reported", open: 3, close: 2, wantPoints: 1},
		{name: "all closed", open: 2, close: 2, wantPoints: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := useMeterProvider(t)

			dbs := make([]*DB, 0, tt.open)
			for i := range tt.open {
				// a host per database so each one is reported with its own attributes,
				// opened without ping as the hosts do not exist.
				db, err := open(fmt.Sprintf("postgresql://user:pass@host-%d/db", i))
				require.NoError(t, err)
				dbs = append(dbs, db)
			}
			t.Cleanup(func() {
				for _, db := range dbs[tt.close:] {
					assert.NoError(t, db.Close())
				}
			})
			for _, db := range dbs[:tt.close] {
				require.NoError(t, db.Close())
				assert.Empty(t, db.stats.regs)
				// closing again is a no-op.
				require.NoError(t, db.Close())
			}

			assert.Equal(t, tt.wantPoints, maxOpenConnsPoints(t, reader))
		})
	}
}

func TestOpenReleasesMetrics(t *testing.T) {
	tests := []struct {
		name       string
		connection func(t *testing.T) string
		opts       []Option
	}{
		{name: "unreachable database", connection: unreachableDSN},
		{
			name:       "invalid option",
			connection: func(*testing.T) string { return "postgresql://user:pass@localhost/db" },
			opts:       []Option{WithStatementCacheMode("unknown")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := useMeterProvider(t)

			db, err := Open(context.Background(), tt.connection(t), tt.opts...)
			require.Error(t, err)
			assert.Nil(t, db)
			assert.Equal(t, 0, maxOpenConnsPoints(t, reader))
		})
	}
}

func TestTestingDBReleasesMetrics(t *testing.T) {
	reader := useMeterProvider(t)

	tdb := openTestingDB(t)
	// the root database used to create the testing database is closed.
	assert.Equal(t, 1, maxOpenConnsPoints(t, reader))

	require.NoError(t, tdb.Close())
	assert.Equal(t, 0, maxOpenConnsPoints(t, reader))
}
