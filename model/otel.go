package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"go.opentelemetry.io/otel"
)

var packageName = fmt.Sprintf("%s%s", evergreen.PackageName, "/model")

var tracer = otel.GetTracerProvider().Tracer(packageName)
