package model

import "go.opentelemetry.io/otel"

const packageName = "github.com/evergreen-ci/evergreen/model"

var tracer = otel.GetTracerProvider().Tracer(packageName)
