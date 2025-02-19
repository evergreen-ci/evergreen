package taskoutput

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"go.opentelemetry.io/otel"
)

var tracer = otel.GetTracerProvider().Tracer(fmt.Sprintf("%s%s", evergreen.PackageName, "/agent/internal/taskoutput"))
