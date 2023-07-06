package units

import "go.opentelemetry.io/otel"

const packageName = "github.com/evergreen-ci/evergreen/units"

var tracer = otel.GetTracerProvider().Tracer(packageName)
