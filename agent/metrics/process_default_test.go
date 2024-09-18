//go:build !windows

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

func TestProcessMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reader := sdk.NewManualReader()
	meter := sdk.NewMeterProvider(sdk.WithReader(reader)).Meter("")
	assert.NoError(t, addProcessMetrics(meter))
	var metrics metricdata.ResourceMetrics
	assert.NoError(t, reader.Collect(ctx, &metrics))
	require.NotEmpty(t, metrics.ScopeMetrics)
	require.Len(t, metrics.ScopeMetrics[0].Metrics, 4)
	assert.Equal(t, fmt.Sprintf("%s.sleeping", processCountPrefix), metrics.ScopeMetrics[0].Metrics[1].Name)
	require.NotEmpty(t, metrics.ScopeMetrics[0].Metrics[1].Data.(metricdata.Sum[int64]).DataPoints)
	assert.NotZero(t, metrics.ScopeMetrics[0].Metrics[1].Data.(metricdata.Sum[int64]).DataPoints[0].Value)
}
