package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func runJasperProcessWithContainer(ctx context.Context, opts *options.Create, commandName, workDir string, failureMetadataTags []string, conf *internal.TaskConfig, manager jasper.Manager, background bool, logger client.LoggerProducer, taskID string, backgroundFailures chan<- internal.BackgroundFailure, continueOnError bool, backgroundCommandFailureEnabled bool) (jasper.Process, error) {
	processCtx := ctx
	var span trace.Span
	if conf.Distro != nil && conf.ContainerID != "" {
		processCtx, span = otel.Tracer("github.com/evergreen-ci/evergreen/agent").Start(ctx, "container.exec_wrap")
		span.SetAttributes(
			attribute.String("container.id", conf.ContainerID),
			attribute.String("container.workdir", workDir),
			attribute.String("container.command_name", commandName),
		)
	}
	if conf.ContainerID != "" {
		if err := agentutil.WrapWithContainer(processCtx, opts, conf.ContainerID, workDir, conf.EnvFileHostDir); err != nil {
			if span != nil {
				span.SetStatus(codes.Error, err.Error())
				span.End()
			}
			return nil, errors.Wrap(err, "wrapping command for container execution")
		}
	}
	proc, err := runJasperProcess(processCtx, manager, background, opts, commandName, taskID, failureMetadataTags, logger, backgroundFailures, continueOnError, backgroundCommandFailureEnabled)
	if span != nil {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
	return proc, err
}
