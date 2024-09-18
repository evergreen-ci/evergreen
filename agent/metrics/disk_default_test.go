//go:build !windows

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestDiskMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reader := sdk.NewManualReader()
	meter := sdk.NewMeterProvider(sdk.WithReader(reader)).Meter("")
	assert.NoError(t, addDiskMetrics(ctx, meter))
	var metrics metricdata.ResourceMetrics
	assert.NoError(t, reader.Collect(ctx, &metrics))
	require.NotEmpty(t, metrics.ScopeMetrics)
	require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics)
	assert.Regexp(t, `system\.disk\.io\.\w+\.read`, metrics.ScopeMetrics[0].Metrics[0].Name)
	require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints)
	assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
}
