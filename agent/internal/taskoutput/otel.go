package taskoutput

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/sdk/trace"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
)

var tracer = otel.GetTracerProvider().Tracer(fmt.Sprintf("%s%s", evergreen.PackageName, "/agent/internal/taskoutput"))

const traceSuffix = "build/OTelTraces"

const maxLineSize = 1024 * 1024

// otelTraceDirectoryHandler implements automatic task output handling for the
// reserved otel trace directory.
type otelTraceDirectoryHandler struct {
	dir         string
	logger      client.LoggerProducer
	traceClient otlptrace.Client
}

// run finds all the trace files in taskDir, uploads their contents
// to the OTel collector, and deletes the files. The files must be written with
// [OTel JSON protobuf encoding], such as the output of the collector's [file exporter].
//
// [OTel JSON protobuf encoding] https://opentelemetry.io/docs/specs/otel/protocol/otlp/#json-protobuf-encoding
// [file exporter] https://pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter
func (o otelTraceDirectoryHandler) run(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "upload-traces")
	defer span.End()

	files, err := getTraceFiles(o.dir)
	if err != nil {
		return errors.Wrapf(err, "getting trace files for '%s'", o.dir)
	}

	if err = o.traceClient.Start(ctx); err != nil {
		return errors.Wrapf(err, "starting trace client for '%s'", o.dir)
	}
	defer func(traceClient otlptrace.Client, ctx context.Context) {
		err := traceClient.Stop(ctx)
		if err != nil {
			o.logger.Task().Error(errors.Wrapf(err, "stopping trace client for '%s'", o.dir))
		}
	}(o.traceClient, ctx)

	// Process files and upload batches in parallel using a worker pool.
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.GOMAXPROCS(0))

	for _, fileName := range files {
		g.Go(func() error {
			if err := gCtx.Err(); err != nil {
				return errors.Wrap(err, "context canceled before processing file")
			}

			resourceSpans, err := unmarshalTraces(fileName)
			if err != nil {
				return errors.Wrapf(err, "unmarshalling trace file '%s'", fileName)
			}

			spanBatches := batchSpans(resourceSpans, trace.DefaultMaxExportBatchSize)
			for _, batch := range spanBatches {
				if err := gCtx.Err(); err != nil {
					return errors.Wrap(err, "context canceled while uploading traces")
				}
				if err := o.traceClient.UploadTraces(gCtx, batch); err != nil {
					return errors.Wrapf(err, "uploading traces for '%s'", fileName)
				}
			}

			if err := os.Remove(fileName); err != nil {
				return errors.Wrapf(err, "removing trace file '%s'", fileName)
			}
			return nil
		})
	}

	return g.Wait()
}

// newOtelTraceDirectoryHandler returns a new otel trace directory handler for the
// specified task.
func newOtelTraceDirectoryHandler(dir string, logger client.LoggerProducer, handlerOpts directoryHandlerOpts) directoryHandler {
	h := &otelTraceDirectoryHandler{
		dir:         dir,
		logger:      logger,
		traceClient: handlerOpts.traceClient,
	}
	return h
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
func getTraceFiles(traceDir string) ([]string, error) {
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
