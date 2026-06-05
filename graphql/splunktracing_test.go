package graphql

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator/rules"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// userSettingsQuery and hostEventsQuery are real queries taken from the
// graphql/tests directory. They serve as a shallow and a deep query
// respectively, so that the complexity score difference between them is
// meaningful.
const userSettingsQuery = `
query {
  user {
    settings {
      timezone
      region
      githubUser {
        lastKnownAs
      }
      slackUsername
      slackMemberId
    }
  }
}`

const hostEventsQuery = `
query {
  host(hostId: "i-0f81a2d39744003dd") {
    events(opts: {}) {
      eventLogEntries {
        id
        resourceType
        processedAt
        timestamp
        eventType
        data {
          agentRevision
          agentBuild
          oldStatus
          newStatus
          logs
          hostname
          provisioningMethod
          taskId
          taskPid
          taskStatus
          execution
          monitorOp
          user
          successful
          duration
        }
        resourceId
      }
      count
    }
  }
}`

type capturedOutput struct {
	logFields message.Fields
	spanAttrs map[string]any
}

func TestGraphQLComplexityLogging(t *testing.T) {
	schema := NewExecutableSchema(New(""))

	parseOp := func(t *testing.T, queryStr string) *ast.OperationDefinition {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), queryStr, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		require.Len(t, doc.Operations, 1)
		return doc.Operations[0]
	}

	// runAndCapture runs InterceptResponse with a recording OTel span and an
	// internal grip sender, returning the logged Splunk fields and the span
	// attributes so both outputs can be asserted.
	runAndCapture := func(t *testing.T, op *ast.OperationDefinition) capturedOutput {
		// Swap grip sender to capture log output.
		defer func(s send.Sender) {
			assert.NoError(t, grip.SetSender(s))
		}(grip.GetSender())
		sender := send.MakeInternalLogger()
		require.NoError(t, grip.SetSender(sender))

		// Build a recording tracer so span attributes can be inspected.
		spanRecorder := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
		rc := &graphql.OperationContext{Operation: op}
		ctx := graphql.WithOperationContext(t.Context(), rc)
		ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})
		ctx, span := tp.Tracer("test").Start(ctx, "test")

		NewSplunkTracing(schema).InterceptResponse(ctx, func(ctx context.Context) *graphql.Response {
			return &graphql.Response{}
		})
		span.End()

		require.True(t, sender.HasMessage())
		fields, ok := sender.GetMessage().Message.Raw().(message.Fields)
		require.True(t, ok)

		ended := spanRecorder.Ended()
		require.Len(t, ended, 1)
		attrs := map[string]any{}
		for _, kv := range ended[0].Attributes() {
			attrs[string(kv.Key)] = kv.Value.AsInterface()
		}

		return capturedOutput{logFields: fields, spanAttrs: attrs}
	}

	t.Run("SplunkLogsComplexityScore", func(t *testing.T) {
		out := runAndCapture(t, parseOp(t, userSettingsQuery))
		score, ok := out.logFields["complexity_score"].(int)
		require.True(t, ok, "complexity_score should be present in Splunk log fields as int")
		assert.Greater(t, score, 0)
	})

	t.Run("OTelSpanHasComplexityScore", func(t *testing.T) {
		out := runAndCapture(t, parseOp(t, userSettingsQuery))
		val, ok := out.spanAttrs["graphql.complexity_score"]
		require.True(t, ok, "graphql.complexity_score should be present as a span attribute")
		score, ok := val.(int64)
		require.True(t, ok)
		assert.Greater(t, score, int64(0))
	})

	t.Run("BothOutputsAgreeOnScore", func(t *testing.T) {
		out := runAndCapture(t, parseOp(t, userSettingsQuery))
		logScore := int64(out.logFields["complexity_score"].(int))
		spanScore := out.spanAttrs["graphql.complexity_score"].(int64)
		assert.Equal(t, logScore, spanScore)
	})

	t.Run("DeeperQueryScoresHigher", func(t *testing.T) {
		simple := runAndCapture(t, parseOp(t, userSettingsQuery))
		complex := runAndCapture(t, parseOp(t, hostEventsQuery))
		assert.Greater(t, complex.logFields["complexity_score"].(int), simple.logFields["complexity_score"].(int))
		assert.Greater(t, complex.spanAttrs["graphql.complexity_score"].(int64), simple.spanAttrs["graphql.complexity_score"].(int64))
	})
}
