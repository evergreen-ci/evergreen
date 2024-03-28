package operations

import "go.opentelemetry.io/otel"

var tracer = otel.GetTracerProvider().Tracer("github.com/evergreen-ci/evergreen/operations")
