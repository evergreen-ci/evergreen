package validator

import "go.opentelemetry.io/otel"

const packageName = "github.com/evergreen-ci/evergreen/validator"

var tracer = otel.GetTracerProvider().Tracer(packageName)
