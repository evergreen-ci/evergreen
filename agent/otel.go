package agent

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/metrics"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/detectors/aws/ec2"
	"go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	exportInterval = 15 * time.Second
	exportTimeout  = exportInterval * 2
	packageName    = "github.com/evergreen-ci/evergreen/agent"
	traceSuffix    = "build/OTelTraces"
	maxLineSize    = 1024 * 1024
)

func (a *Agent) initOtel(ctx context.Context) error {
	if a.opts.TraceCollectorEndpoint == "" {
		a.tracer = otel.GetTracerProvider().Tracer(packageName)
		return nil
	}

	r, err := hostResource(ctx)
	if err != nil {
		return errors.Wrap(err, "making host resource")
	}

	a.otelGrpcConn, err = grpc.DialContext(ctx,
		a.opts.TraceCollectorEndpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
	)
	if err != nil {
		return errors.Wrapf(err, "opening gRPC connection to '%s'", a.opts.TraceCollectorEndpoint)
	}

	client := otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(a.otelGrpcConn))
	traceExporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return errors.Wrap(err, "initializing otel exporter")
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(r),
	)
	tp.RegisterSpanProcessor(utility.NewAttributeSpanProcessor())
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		grip.Error(errors.Wrap(err, "otel error"))
	}))

	a.tracer = tp.Tracer(packageName)

	a.closers = append(a.closers, closerOp{
		name: "tracer provider shutdown",
		closerFn: func(ctx context.Context) error {
			catcher := grip.NewBasicCatcher()
			catcher.Wrap(tp.Shutdown(ctx), "trace provider shutdown")
			catcher.Wrap(traceExporter.Shutdown(ctx), "trace exporter shutdown")
			catcher.Wrap(a.otelGrpcConn.Close(), "closing gRPC connection")

			return catcher.Resolve()
		},
	})

	return nil
}

func (a *Agent) startMetrics(ctx context.Context, tc *internal.TaskConfig) (func(context.Context), error) {
	if a.otelGrpcConn == nil {
		return nil, nil
	}

	metricsExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(a.otelGrpcConn))
	if err != nil {
		return nil, errors.Wrap(err, "making otel metrics exporter")
	}

	r, err := hostResource(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "making resource")
	}

	r, err = resource.Merge(r, resource.NewSchemaless(tc.TaskAttributes()...))
	if err != nil {
		return nil, errors.Wrap(err, "merging host resource with task attributes")
	}

	meterProvider := sdk.NewMeterProvider(
		sdk.WithResource(r),
		sdk.WithReader(sdk.NewPeriodicReader(metricsExporter, sdk.WithInterval(exportInterval), sdk.WithTimeout(exportTimeout))),
	)

	return func(ctx context.Context) {
		grip.Error(errors.Wrap(meterProvider.Shutdown(ctx), "doing meter provider"))
	}, errors.Wrap(metrics.InstrumentMeter(ctx, meterProvider.Meter(packageName)), "instrumenting meter")
}

func hostResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("evergreen-agent")),
		resource.WithAttributes(semconv.ServiceVersion(evergreen.BuildRevision)),
		resource.WithDetectors(ec2.NewResourceDetector(), ecs.NewResourceDetector()),
	)
}

// uploadTraces finds all the trace files in taskDir, uploads their contents
// to the OTel collector, and deletes the files. The files must be written with
// [OTel JSON protobuf encoding], such as the output of the collector's [file exporter].
//
// [OTel JSON protobuf encoding] https://opentelemetry.io/docs/specs/otel/protocol/otlp/#json-protobuf-encoding
// [file exporter] https://pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter
func (a *Agent) uploadTraces(ctx context.Context, taskDir string) error {
	files, err := getTraceFiles(taskDir)
	if err != nil {
		return errors.Wrapf(err, "getting trace files for '%s'", taskDir)
	}
	client := otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(a.otelGrpcConn))
	if err := client.Start(ctx); err != nil {
		return errors.Wrap(err, "starting trace client")
	}
	defer func() { grip.Error(errors.Wrap(client.Stop(ctx), "stopping trace gRPC client")) }()

	catcher := grip.NewBasicCatcher()
	for _, fileName := range files {
		resourceSpans, err := unmarshalTraces(fileName)
		if err != nil {
			catcher.Wrapf(err, "unmarshalling trace file '%s'", fileName)
			continue
		}

		spanBatches := batchSpans(resourceSpans, sdktrace.DefaultMaxExportBatchSize)
		for _, batch := range spanBatches {
			if err = client.UploadTraces(ctx, batch); err != nil {
				catcher.Wrapf(err, "uploading traces for '%s'", fileName)
				continue
			}
		}

		catcher.Wrapf(os.Remove(fileName), "removing trace file '%s'", fileName)
	}

	return catcher.Resolve()
}

// batchSpans batches spans to avoid exceeding the collector's gRPC message size limit of 4MB.
// Batching algorithm from https://go.dev/wiki/SliceTricks#batching-with-minimal-allocation
func batchSpans(spans []*tracepb.ResourceSpans, batchSize int) [][]*tracepb.ResourceSpans {
	batches := make([][]*tracepb.ResourceSpans, 0, (len(spans)+batchSize-1)/batchSize)

	for batchSize < len(spans) {
		spans, batches = spans[batchSize:], append(batches, spans[0:batchSize:batchSize])
	}
	return append(batches, spans)
}

func unmarshalTraces(fileName string) ([]*tracepb.ResourceSpans, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "opening trace file '%s'", fileName)
	}
	defer func() { grip.Error(errors.Wrapf(file.Close(), "closing trace file '%s'", fileName)) }()

	catcher := grip.NewBasicCatcher()

	var resourceSpans []*tracepb.ResourceSpans
	scanner := bufio.NewScanner(file)
	scanner.Buffer([]byte{}, maxLineSize)
	for scanner.Scan() {
		var traces tracepb.TracesData
		catcher.Wrap(protojson.Unmarshal(scanner.Bytes(), &traces), "unmarshalling trace")
		resourceSpans = append(resourceSpans, traces.ResourceSpans...)
	}
	if err := scanner.Err(); err != nil {
		catcher.Wrapf(err, "scanning file '%s'", fileName)
	}

	if err = fixBinaryIDs(resourceSpans); err != nil {
		return nil, errors.Wrapf(err, "fixing binary IDs for '%s'", fileName)
	}

	return resourceSpans, catcher.Resolve()
}

// fixBinaryIDs fixes every trace and span id in resourceSpans. These IDs are encoded
// as hex strings in the source file because that's how the [OTel JSON protobuf encoding] is defined
// but [protojson] assumes they're encoded with base64 encoding since that's the [standard JSON encoding].
// We need to iterate through the spans and fix them.
//
// [OTel JSON protobuf encoding]: https://opentelemetry.io/docs/specs/otel/protocol/otlp/#json-protobuf-encoding
// [standard JSON encoding]: https://protobuf.dev/programming-guides/proto3/#json
func fixBinaryIDs(resourceSpans []*tracepb.ResourceSpans) error {
	catcher := grip.NewBasicCatcher()
	for _, rs := range resourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, span := range ss.Spans {
				catcher.Wrap(fixSpan(span), "fixing span")
				for _, spanLink := range span.Links {
					catcher.Add(fixSpanLink(spanLink))
				}
			}
		}
	}

	return catcher.Resolve()
}

func fixSpan(span *tracepb.Span) error {
	traceIDHex, err := fixBinaryID(span.TraceId)
	if err != nil {
		return errors.Wrap(err, "fixing trace id")
	}
	spanIDHex, err := fixBinaryID(span.SpanId)
	if err != nil {
		return errors.Wrap(err, "fixing span id")
	}
	parentSpanIDHex, err := fixBinaryID(span.ParentSpanId)
	if err != nil {
		return errors.Wrap(err, "fixing parent span id")
	}

	span.TraceId = traceIDHex
	span.SpanId = spanIDHex
	span.ParentSpanId = parentSpanIDHex
	return nil
}

func fixSpanLink(spanLink *tracepb.Span_Link) error {
	traceIDHex, err := fixBinaryID(spanLink.TraceId)
	if err != nil {
		return errors.Wrap(err, "fixing trace id")
	}
	spanIDHex, err := fixBinaryID(spanLink.SpanId)
	if err != nil {
		return errors.Wrap(err, "fixing span id")
	}

	spanLink.TraceId = traceIDHex
	spanLink.SpanId = spanIDHex
	return nil
}

// fixBinaryID recovers the original hex string id and decodes it back
// into []byte. The unmarshaller decoded the string as a base64 encoded
// string so we encode it back to the string and decode it again to []byte.
func fixBinaryID(id []byte) ([]byte, error) {
	idHex := base64.StdEncoding.EncodeToString(id)
	return hex.DecodeString(idHex)
}

// getTraceFiles returns the full path of all the files in the [traceSuffix] directory
// under the task's working directory.
func getTraceFiles(taskDir string) ([]string, error) {
	traceDir := path.Join(taskDir, traceSuffix)
	info, err := os.Stat(traceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "getting info on '%s'", traceDir)
	}
	if !info.IsDir() {
		return nil, nil
	}

	files, err := os.ReadDir(traceDir)
	if err != nil {
		return nil, errors.Wrapf(err, "getting files from '%s'", traceDir)
	}

	var fileNames []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileNames = append(fileNames, path.Join(traceDir, file.Name()))
	}

	return fileNames, nil
}
