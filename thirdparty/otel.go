package thirdparty

import "go.opentelemetry.io/otel"

const packageName = "github.com/evergreen-ci/evergreen/thirdparty"

var tracer = otel.GetTracerProvider().Tracer(packageName)
