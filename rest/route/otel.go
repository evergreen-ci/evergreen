package route

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"go.opentelemetry.io/otel"
)

var packageName = fmt.Sprintf("%s%s", evergreen.PackageName, "/route")

var tracer = otel.GetTracerProvider().Tracer(packageName)
