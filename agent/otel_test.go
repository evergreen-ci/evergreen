package agent

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	testCases := map[string]func(t *testing.T, meter metric.Meter, reader sdk.Reader){
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
			assert.NoError(t, addNetworkMetrics(t.Context(), meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.NotEmpty(t, metrics.ScopeMetrics)
			require.Len(t, metrics.ScopeMetrics[0].Metrics, 2)
			assert.Equal(t, fmt.Sprintf("%s.transmit", networkIOInstrumentPrefix), metrics.ScopeMetrics[0].Metrics[0].Name)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints)
			assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
		},
	}

	if runtime.GOOS != "darwin" {
		testCases["CPUMetrics"] = func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
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
		testCases["DiskMetrics"] = func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
			assert.NoError(t, addDiskMetrics(ctx, meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.NotEmpty(t, metrics.ScopeMetrics)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics)
			assert.Regexp(t, `system\.disk\.io\.\w+\.read`, metrics.ScopeMetrics[0].Metrics[0].Name)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints)
			assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
		}
	}

	if runtime.GOOS != "windows" {
		testCases["ProcessMetrics"] = func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
			assert.NoError(t, addProcessMetrics(meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.NotEmpty(t, metrics.ScopeMetrics)
			require.Len(t, metrics.ScopeMetrics[0].Metrics, 4)
			assert.Equal(t, fmt.Sprintf("%s.sleeping", processCountPrefix), metrics.ScopeMetrics[0].Metrics[1].Name)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[1].Data.(metricdata.Sum[int64]).DataPoints)
			assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[1].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
		}
	} else {
		testCases["ProcessMetrics"] = func(t *testing.T, meter metric.Meter, reader sdk.Reader) {
			assert.NoError(t, addProcessMetrics(meter))
			var metrics metricdata.ResourceMetrics
			assert.NoError(t, reader.Collect(ctx, &metrics))
			require.NotEmpty(t, metrics.ScopeMetrics)
			require.Len(t, metrics.ScopeMetrics[0].Metrics, 1)
			assert.Equal(t, processCountPrefix, metrics.ScopeMetrics[0].Metrics[0].Name)
			require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints)
			assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
		}
	}

	for testName, testCase := range testCases {
		reader := sdk.NewManualReader()
		meter := sdk.NewMeterProvider(sdk.WithReader(reader)).Meter(packageName)
		t.Run(testName, func(t *testing.T) { testCase(t, meter, reader) })
	}
}
