# Task Traces

Evergreen creates a trace for each task execution.
![task_trace.png](../images/task_trace.png)

A task's execution is decomposed to the level of commands. If a command 
encounters an error the error is recorded as a [span link](https://opentelemetry.io/docs/concepts/signals/traces/#span-links) on the span.

## Sending traces
Evergreen provides two ways to send traces to the trace backend, 1) an [OTel collector](#otel-collector) and 2) [trace file parsing](#trace-file-parsing)

### OTel collector
Evergreen provides an [OTel collector](https://opentelemetry.io/docs/collector/) to capture traces. The gRPC endpoint is exposed to tasks as the `${otel_collector_endpoint}` [default expansion](Project-Configuration-Files.md#default-expansions).

### Trace file parsing
When a task finishes Evergreen will check if any [encoded](#json-protobuf-encoding) trace files have been written to a `{task_working_directory}/build/trace` directory, parse them, and send them to the collector. This can be useful if a test is running without a connection to the network. Only trace files are supported, and OTel metrics/logs files in the directory will be skipped.

#### JSON protobuf encoding 
OTel defines [JSON protobuf encoding](https://opentelemetry.io/docs/specs/otel/protocol/otlp/#json-protobuf-encoding) for serializing traces to files. Some OTel SDKs support this natively (e.g. the Java SDK provides the [OtlpJsonLoggingSpanExporter exporter](https://javadoc.io/static/io.opentelemetry/opentelemetry-exporter-logging-otlp/1.10.0-rc.2/io/opentelemetry/exporter/logging/otlp/OtlpJsonLoggingSpanExporter.html)). If this isn't an option (e.g. the SDK for go doesn't provide a JSON protobuf exporter) another option is to configure the test to send its traces to a local collector running alongside the test and configure the collector to use the [file exporter](https://pkg.go.dev/github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter). The file exporter is only available in [the collector's "contrib" distribution](https://github.com/open-telemetry/opentelemetry-collector-contrib) (release builds for many OS/architectures are available [here](https://github.com/open-telemetry/opentelemetry-collector-releases/releases)).

## Hooking tests into command spans
Evergreen exposes every command's trace and span IDs to a running command as hex encoded strings in the `${otel_trace_id}` and `${otel_parent_id}` [default expansions](Project-Configuration/Project-Configuration-Files.md#default-expansions). To hook a test's spans into the command's span the trace id and parent id can be added to the current context.

### Language specific examples
The following examples illustrate how to inject the trace/span IDs into the context. They assume the script has the `${otel_trace_id}` and `${otel_parent_id}` expansions expanded.

#### Python
```python
from opentelemetry import trace, context
from opentelemetry.trace import NonRecordingSpan, SpanContext

span_context = SpanContext(
    trace_id = int("${otel_trace_id}", 16),
    span_id = int(("${otel_parent_id}", 16)
)
ctx = trace.set_span_in_context(NonRecordingSpan(span_context))

# Now there are a few ways to make use of the trace context.

# You can pass the context object when starting a span.
with tracer.start_as_current_span('child', context=ctx) as span:
    span.set_attribute('primes', [2, 3, 5, 7])

# Or you can make it the current context, and then the next span will pick it up.
# The returned token lets you restore the previous context.
token = context.attach(ctx)
try:
    with tracer.start_as_current_span('child') as span:
        span.set_attribute('evens', [2, 4, 6, 8])
finally:
    context.detach(token)

```
(adapted from [here](https://opentelemetry.io/docs/instrumentation/python/cookbook/#manually-setting-span-context))

#### Go
```go
import "go.opentelemetry.io/otel/trace"

traceID, err := trace.TraceIDFromHex("${otel_trace_id}")
if err != nil {
    return err
}
spanID, err := trace.SpanIDFromHex("${otel_parent_id}")
if err != nil {
    return err
}

// Create a span context from the provided trace/span ids.
sc := trace.NewSpanContext(SpanContextConfig{
    TraceID: traceID,
    SpanID: spanID,
})

// Inject the span context into a context.
ctx = trace.ContextWithSpanContext(ctx, sc)

// Use the ctx when creating the each of the test's root spans.
ctx, span = tracer.Start(ctx, "test_span")
defer span.End()
```