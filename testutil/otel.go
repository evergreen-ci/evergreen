package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	testCtxMap  = make(map[string]context.Context)
	packageName = fmt.Sprintf("%s%s", evergreen.PackageName, "/testutil")
)

func TestSpan(t *testing.T) context.Context {
	if parentCtx := contextForTest(t); parentCtx == nil {
		return spanForRootTest(t)
	} else {
		return spanForChildTest(parentCtx, t)
	}
}

func contextForTest(t *testing.T) context.Context {
	testName := t.Name()
	sep := "/"
	tests := strings.Split(testName, sep)

	var ctx context.Context
	for x := len(tests); x >= 0; x-- {
		testCtx, ok := testCtxMap[strings.Join(tests[:x], sep)]
		if ok {
			ctx = testCtx
			break
		}
	}

	return ctx
}

func spanForRootTest(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	collectorEndpoint := os.Getenv(otelCollectorEndpointEnvVar)
	traceIDString := os.Getenv(otelTraceIDEnvVar)
	spanIDString := os.Getenv(otelParentIDEnvVar)
	if collectorEndpoint == "" || traceIDString == "" || spanIDString == "" {
		return nil
	}

	tracerCloser, err := initTracer(ctx, collectorEndpoint)
	if err != nil {
		grip.Error(errors.Wrap(err, "initializing tracer provider"))
		return nil
	}

	traceID, err := trace.TraceIDFromHex(traceIDString)
	if err != nil {
		grip.Error(errors.Wrapf(err, "parsing trace ID '%s'", traceIDString))
		return nil
	}
	spanID, err := trace.SpanIDFromHex(spanIDString)
	if err != nil {
		grip.Error(errors.Wrapf(err, "parsing parent span ID '%s'", spanIDString))
		return nil
	}
	parentCtx := trace.ContextWithSpanContext(
		ctx,
		trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: trace.FlagsSampled,
		}),
	)

	testCtx, span := otel.GetTracerProvider().Tracer(packageName).Start(parentCtx, t.Name())
	testCtxMap[t.Name()] = testCtx

	t.Cleanup(func() {
		span.End()
		if err := tracerCloser(ctx); err != nil {
			grip.Error(errors.Wrap(tracerCloser(ctx), "closing otel tracer"))
		}
	})

	return testCtx
}

func spanForChildTest(parentCtx context.Context, t *testing.T) context.Context {
	testCtx, span := otel.GetTracerProvider().Tracer(packageName).Start(parentCtx, t.Name())
	testCtxMap[t.Name()] = testCtx

	t.Cleanup(func() {
		span.End()
	})

	return testCtx
}

func initTracer(ctx context.Context, collectorEndpoint string) (func(context.Context) error, error) {
	resource := resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("evergreen-tests"))
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(collectorEndpoint),
	)
	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, errors.Wrap(err, "initializing otel exporter")
	}

	spanLimits := sdktrace.NewSpanLimits()
	spanLimits.AttributeValueLengthLimit = evergreen.OtelAttributeMaxLength

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource),
		sdktrace.WithRawSpanLimits(spanLimits),
	)
	tp.RegisterSpanProcessor(utility.NewAttributeSpanProcessor())
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		grip.Error(errors.Wrap(err, "otel error"))
	}))

	return func(ctx context.Context) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(tp.Shutdown(ctx))
		catcher.Add(exp.Shutdown(ctx))
		return nil
	}, nil
}
