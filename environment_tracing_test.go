package evergreen

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/anser/apm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestMongoCommandTracingShouldPreserveSpansWithoutLargeBodies(t *testing.T) {
	for _, test := range []struct {
		name       string
		collection string
		payload    string
		keepBody   bool
	}{
		{name: "SmallCommandShouldRetainBody", collection: "tasks", payload: "small", keepBody: true},
		{name: "LargeCommandShouldOmitBody", collection: "tasks", payload: strings.Repeat("x", 64*1024)},
		{name: "SensitiveCollectionShouldOmitBody", collection: "events", payload: "sensitive"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.WithoutCancel(t.Context()))) })
			monitor := apm.NewMonitor(apm.WithTracerProvider(provider), apm.WithCommandAttributeDisabled(false), apm.WithCommandAttributeTransformer(redactSensitiveCollections))
			command, err := bson.Marshal(bson.D{
				{Key: "insert", Value: test.collection},
				{Key: "documents", Value: bson.A{bson.D{{Key: "payload", Value: test.payload}}}},
			})
			require.NoError(t, err)
			monitor.Started(t.Context(), &event.CommandStartedEvent{
				Command: command, CommandName: "insert", DatabaseName: "test", ConnectionID: "localhost:27017", RequestID: 1,
			})
			monitor.Succeeded(t.Context(), &event.CommandSucceededEvent{
				CommandFinishedEvent: event.CommandFinishedEvent{CommandName: "insert", ConnectionID: "localhost:27017", RequestID: 1, Duration: time.Millisecond},
			})
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			assert.Equal(t, test.collection+".insert", spans[0].Name())
			attributes := map[attribute.Key]attribute.Value{}
			for _, value := range spans[0].Attributes() {
				attributes[value.Key] = value.Value
			}
			assert.Equal(t, "insert", attributes["db.operation"].AsString())
			assert.Equal(t, test.collection, attributes["db.mongodb.collection"].AsString())
			_, hasStatement := attributes["db.statement"]
			assert.Equal(t, test.keepBody, hasStatement)
			if !test.keepBody {
				for key := range attributes {
					assert.NotContains(t, string(key), "statement")
				}
			}
		})
	}
}
