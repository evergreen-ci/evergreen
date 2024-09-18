package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCPUMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reader := sdk.NewManualReader()
	meter := sdk.NewMeterProvider(sdk.WithReader(reader)).Meter("")
	assert.NoError(t, addCPUMetrics(meter))
	var metrics metricdata.ResourceMetrics
	assert.NoError(t, reader.Collect(ctx, &metrics))
	require.NotEmpty(t, metrics.ScopeMetrics)
	require.Len(t, metrics.ScopeMetrics[0].Metrics, 6)
	assert.Equal(t, fmt.Sprintf("%s.idle", cpuTimeInstrumentPrefix), metrics.ScopeMetrics[0].Metrics[0].Name)
	require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[float64]).DataPoints)
	assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[float64]).DataPoints[0].Value)

	assert.Equal(t, cpuUtilInstrument, metrics.ScopeMetrics[0].Metrics[5].Name)
	require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[5].Data.(metricdata.Gauge[float64]).DataPoints)
	assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[5].Data.(metricdata.Gauge[float64]).DataPoints[0].Value)
}
