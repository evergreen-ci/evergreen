package agent

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.opentelemetry.io/contrib/detectors/aws/ec2"
	"go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
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

	cpuTimeInstrumentPrefix = "system.cpu.time"
	cpuUtilInstrument       = "system.cpu.utilization"

	memoryUsageInstrumentPrefix = "system.memory.usage"
	memoryUtilizationInstrument = "system.memory.utilization"

	diskIOInstrumentPrefix         = "system.disk.io"
	diskOperationsInstrumentPrefix = "system.disk.operations"
	diskIOTimeInstrumentPrefix     = "system.disk.io_time"

	networkIOInstrumentPrefix = "system.network.io"

	processCountPrefix = "system.process.count"
)

var instrumentNameDisallowedCharacters = regexp.MustCompile(`[^A-Za-z0-9_.-/]`)

func (a *Agent) initOtel(ctx context.Context) error {
	if a.opts.TraceCollectorEndpoint == "" {
		a.tracer = otel.GetTracerProvider().Tracer(packageName)
		return nil
	}

	var err error
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
		sdktrace.WithResource(hostResource(ctx)),
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

	r, err := resource.Merge(hostResource(ctx), resource.NewSchemaless(tc.TaskAttributes()...))
	if err != nil {
		return nil, errors.Wrap(err, "merging host resource with task attributes")
	}

	meterProvider := sdk.NewMeterProvider(
		sdk.WithResource(r),
		sdk.WithReader(sdk.NewPeriodicReader(metricsExporter, sdk.WithInterval(exportInterval), sdk.WithTimeout(exportTimeout))),
	)

	return func(ctx context.Context) {
		grip.Error(errors.Wrap(meterProvider.Shutdown(ctx), "doing meter provider"))
	}, errors.Wrap(instrumentMeter(ctx, meterProvider.Meter(packageName)), "instrumenting meter")
}

func instrumentMeter(ctx context.Context, meter metric.Meter) error {
	catcher := grip.NewBasicCatcher()

	// CPU and disk metrics are not implemented by gopsutil for macOS.
	if runtime.GOOS != "darwin" {
		catcher.Wrap(addCPUMetrics(meter), "adding CPU metrics")
		catcher.Wrap(addDiskMetrics(ctx, meter), "adding disk metrics")
	}

	catcher.Wrap(addMemoryMetrics(meter), "adding memory metrics")
	catcher.Wrap(addNetworkMetrics(meter), "adding network metrics")
	catcher.Wrap(addProcessMetrics(meter), "adding process metrics")

	return catcher.Resolve()
}

func addCPUMetrics(meter metric.Meter) error {
	cpuTimeIdle, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.idle", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time idle counter")
	}
	cpuTimeSystem, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.system", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time system counter")
	}
	cpuTimeUser, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.user", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time user counter")
	}
	cpuTimeSteal, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.steal", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time steal counter")
	}
	cpuTimeIOWait, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.iowait", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time iowait counter")
	}

	cpuUtil, err := meter.Float64ObservableGauge(cpuUtilInstrument, metric.WithUnit("1"), metric.WithDescription("Busy CPU time since the last measurement, divided by the elapsed time"))
	if err != nil {
		return errors.Wrap(err, "making cpu util gauge")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		times, err := cpu.TimesWithContext(ctx, false)
		if err != nil {
			return errors.Wrap(err, "getting CPU times")
		}
		if len(times) != 1 {
			return errors.Wrap(err, "CPU times had an unexpected length")
		}
		observer.ObserveFloat64(cpuTimeIdle, times[0].Idle)
		observer.ObserveFloat64(cpuTimeSystem, times[0].System)
		observer.ObserveFloat64(cpuTimeUser, times[0].User)
		observer.ObserveFloat64(cpuTimeSteal, times[0].Steal)
		observer.ObserveFloat64(cpuTimeIOWait, times[0].Iowait)

		return nil
	}, cpuTimeIdle, cpuTimeSystem, cpuTimeUser, cpuTimeSteal, cpuTimeIOWait)
	if err != nil {
		return errors.Wrap(err, "registering cpu time callback")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		util, err := cpu.PercentWithContext(ctx, 0, false)
		if err != nil {
			return errors.Wrap(err, "getting CPU util")
		}
		if len(util) != 1 {
			return errors.Wrap(err, "CPU util had an unexpected length")
		}
		observer.ObserveFloat64(cpuUtil, util[0])

		return nil
	}, cpuUtil)
	return errors.Wrap(err, "registering cpu time callback")
}

func addMemoryMetrics(meter metric.Meter) error {
	memoryUsageAvailable, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.available", memoryUsageInstrumentPrefix), metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making memory usage available counter")
	}

	memoryUsageUsed, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.used", memoryUsageInstrumentPrefix), metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making memory usage used counter")
	}

	memoryUtil, err := meter.Float64ObservableGauge(memoryUtilizationInstrument, metric.WithUnit("1"))
	if err != nil {
		return errors.Wrap(err, "making memory util gauge")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		memStats, err := mem.VirtualMemoryWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting memory stats")
		}
		observer.ObserveInt64(memoryUsageAvailable, int64(memStats.Available))
		observer.ObserveInt64(memoryUsageUsed, int64(memStats.Used))
		observer.ObserveFloat64(memoryUtil, memStats.UsedPercent)

		return nil
	}, memoryUsageAvailable, memoryUsageUsed, memoryUtil)
	return errors.Wrap(err, "registering memory callback")
}

func addDiskMetrics(ctx context.Context, meter metric.Meter) error {
	ioCountersMap, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return errors.Wrap(err, "getting disk stats")
	}

	type diskInstruments struct {
		diskIORead          metric.Int64ObservableCounter
		diskIOWrite         metric.Int64ObservableCounter
		diskOperationsRead  metric.Int64ObservableCounter
		diskOperationsWrite metric.Int64ObservableCounter
		diskIOTime          metric.Float64ObservableCounter
	}
	diskInstrumentMap := map[string]diskInstruments{}
	var allInstruments []metric.Observable
	for diskName := range ioCountersMap {
		// Instrument names may only contain characters in the allowed set. Characters such as : (such as in C: on Windows) are disallowed.
		sanitizedDiskName := instrumentNameDisallowedCharacters.ReplaceAllString(diskName, "")

		diskIORead, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.read", diskIOInstrumentPrefix, sanitizedDiskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk io read counter for disk '%s'", diskName)
		}
		diskIOWrite, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.write", diskIOInstrumentPrefix, sanitizedDiskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk io write counter for disk '%s'", diskName)
		}

		diskOperationsRead, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.read", diskOperationsInstrumentPrefix, sanitizedDiskName), metric.WithUnit("{operation}"))
		if err != nil {
			return errors.Wrapf(err, "making disk operations read counter for disk '%s'", diskName)
		}
		diskOperationsWrite, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.write", diskOperationsInstrumentPrefix, sanitizedDiskName), metric.WithUnit("{operation}"))
		if err != nil {
			return errors.Wrapf(err, "making disk operations write counter for disk '%s'", diskName)
		}

		diskIOTime, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.%s", diskIOTimeInstrumentPrefix, sanitizedDiskName), metric.WithUnit("s"), metric.WithDescription("Time disk spent activated"))
		if err != nil {
			return errors.Wrapf(err, "making disk io time counter for disk '%s'", diskName)
		}

		diskInstrumentMap[diskName] = diskInstruments{
			diskIORead:          diskIORead,
			diskIOWrite:         diskIOWrite,
			diskOperationsRead:  diskOperationsRead,
			diskOperationsWrite: diskOperationsWrite,
			diskIOTime:          diskIOTime,
		}
		allInstruments = append(allInstruments, diskIORead, diskIOWrite, diskOperationsRead, diskOperationsWrite, diskIOTime)
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		ioCountersMap, err := disk.IOCountersWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting disk stats")
		}
		for diskName, instruments := range diskInstrumentMap {
			counter, ok := ioCountersMap[diskName]
			if !ok {
				// If the disk is no longer present there are no readings for it.
				return nil
			}
			observer.ObserveInt64(instruments.diskIORead, int64(counter.ReadBytes))
			observer.ObserveInt64(instruments.diskIOWrite, int64(counter.WriteBytes))

			observer.ObserveInt64(instruments.diskOperationsRead, int64(counter.ReadCount))
			observer.ObserveInt64(instruments.diskOperationsWrite, int64(counter.WriteCount))

			observer.ObserveFloat64(instruments.diskIOTime, float64(counter.IoTime))
		}
		return nil
	}, allInstruments...)
	if err != nil {
		return errors.Wrapf(err, "registering callbacks for disk metrics")
	}

	return nil
}

func addNetworkMetrics(meter metric.Meter) error {
	networkIOTransmit, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.transmit", networkIOInstrumentPrefix), metric.WithUnit("by"))
	if err != nil {
		return errors.Wrap(err, "making network io transmit counter")
	}

	networkIOReceive, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.receive", networkIOInstrumentPrefix), metric.WithUnit("by"))
	if err != nil {
		return errors.Wrap(err, "making network io receive counter")
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		counters, err := net.IOCountersWithContext(ctx, false)
		if err != nil {
			return errors.Wrap(err, "getting network stats")
		}
		if len(counters) != 1 {
			return errors.Wrap(err, "network counters had an unexpected length")
		}

		observer.ObserveInt64(networkIOTransmit, int64(counters[0].BytesSent))
		observer.ObserveInt64(networkIOReceive, int64(counters[0].BytesRecv))

		return nil
	}, networkIOTransmit, networkIOReceive)
	return errors.Wrap(err, "registering network io callback")
}

func hostResource(ctx context.Context) *resource.Resource {
	r := resource.NewSchemaless(
		semconv.ServiceName("evergreen-agent"),
		semconv.ServiceVersion(evergreen.BuildRevision),
	)

	mergedResource, err := addEnvironmentAttributes(ctx, r)
	grip.Error(errors.Wrap(err, "adding environment attributes"))
	if err == nil {
		r = mergedResource
	}

	return r
}

// addEnvironmentAttributes adds attributes to the resource about the environment itself. When running in EC2
// this includes information like the instance id and when running in ECS this includes information like the
// container name. This will noop if not running in EC2/ECS.
func addEnvironmentAttributes(ctx context.Context, r *resource.Resource) (*resource.Resource, error) {
	for name, detector := range map[string]resource.Detector{
		"ec2": ec2.NewResourceDetector(),
		"ecs": ecs.NewResourceDetector(),
	} {
		detectedResource, err := detector.Detect(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "detecting resource '%s'", name)
		}
		mergedResource, err := resource.Merge(r, detectedResource)
		if err != nil {
			return nil, errors.Wrapf(err, "merging resource for detector '%s'", name)
		}
		r = mergedResource
	}

	return r, nil
}

// uploadTraces finds all the trace files in taskDir, uploads their contents
// to the OTel collector, and deletes the files. The files must be written with
// [OTel JSON protobuf encoding], such as the output of the collector's [file exporter].
//
// [OTel JSON protobuf encoding] https://opentelemetry.io/docs/specs/otel/protocol/otlp/#json-protobuf-encoding
// [file exporter] https://pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter
func (a *Agent) uploadTraces(ctx context.Context, taskDir string) error {
	if a.otelGrpcConn == nil {
		return errors.New("OTel gRPC connection has not been configured")
	}

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
