package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, meter metric.Meter, reader sdk.Reader){
		"MemoryMetrics": func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
			assert.NoError(t, addMemoryMetrics(meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.NotEmpty(t, metrics.ScopeMetrics)
			require.Len(t, metrics.ScopeMetrics[0].Metrics, 3)
			assert.Equal(t, fmt.Sprintf("%s.available", memoryUsageInstrumentPrefix), metrics.ScopeMetrics[0].Metrics[0].Name)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints)
			assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
		},
		"NetworkMetrics": func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
			assert.NoError(t, addNetworkMetrics(meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.NotEmpty(t, metrics.ScopeMetrics)
			require.Len(t, metrics.ScopeMetrics[0].Metrics, 2)
			assert.Equal(t, fmt.Sprintf("%s.transmit", networkIOInstrumentPrefix), metrics.ScopeMetrics[0].Metrics[0].Name)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints)
			assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
		},
	} {
		reader := sdk.NewManualReader()
		meter := sdk.NewMeterProvider(sdk.WithReader(reader)).Meter("")
		t.Run(testName, func(t *testing.T) { testCase(t, meter, reader) })
	}
}
