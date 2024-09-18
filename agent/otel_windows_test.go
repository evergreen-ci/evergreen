//go:build windows

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAdditionalMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, meter metric.Meter, reader sdk.Reader){
		"DiskMetrics": func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
			assert.NoError(t, addDiskMetrics(ctx, meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.Empty(t, metrics.ScopeMetrics)
		},
		"ProcessMetrics": func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
			assert.NoError(t, addProcessMetrics(meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.NotEmpty(t, metrics.ScopeMetrics)
			require.Len(t, metrics.ScopeMetrics[0].Metrics, 1)
			assert.Equal(t, processCountPrefix, metrics.ScopeMetrics[0].Metrics[0].Name)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints)
			assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
		},
	} {
		reader := sdk.NewManualReader()
		meter := sdk.NewMeterProvider(sdk.WithReader(reader)).Meter(packageName)
		t.Run(testName, func(t *testing.T) { testCase(t, meter, reader) })
	}
}
