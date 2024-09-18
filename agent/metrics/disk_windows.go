//go:build windows

package metrics

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

// addDiskMetrics is not supported for Windows.
func addDiskMetrics(_ context.Context, _ metric.Meter) error {
	return nil
}
