package githubapp

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"go.opentelemetry.io/otel"
)

var packageName = fmt.Sprintf("%s%s", evergreen.PackageName, "/model/githubapp")

var tracer = otel.GetTracerProvider().Tracer(packageName)
