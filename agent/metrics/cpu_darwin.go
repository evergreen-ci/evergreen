//go:build darwin

package metrics

import "go.opentelemetry.io/otel/metric"

// addCPUMetrics is not supported for macOS.
func addCPUMetrics(_ metric.Meter) error {
	return nil
}
