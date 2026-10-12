package route

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTasksByBuildHandlerPresign(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, artifact.Collection))

	const buildID = "build1"
	tsk := task.Task{Id: "task1", BuildId: buildID}
	require.NoError(t, tsk.Insert(t.Context()))
	entry := artifact.Entry{
		TaskId:  "task1",
		BuildId: buildID,
		Files: []artifact.File{
			{
				Name:       "signed_file",
				Link:       "https://bucket.s3.amazonaws.com/path/to/file",
				Visibility: artifact.Signed,
				Bucket:     "test-bucket",
				FileKey:    "path/to/file",
				AWSKey:     "key",
				AWSSecret:  "secret",
			},
		},
	}
	require.NoError(t, entry.Upsert(t.Context()))

	settings := evergreen.GetEnvironment().Settings()
	originalSecret := settings.ArtifactSignSecret
	originalUIURL := settings.Ui.Url
	t.Cleanup(func() {
		settings.ArtifactSignSecret = originalSecret
		settings.Ui.Url = originalUIURL
	})
	settings.Ui.Url = "http://evergreen.example.net"

	runRequest := func(t *testing.T, query string) (string, map[attribute.Key]string) {
		spanRecorder := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
		ctx, span := tp.Tracer("test").Start(withRequestHost(t.Context(), "evergreen.example.net"), "request")

		r, err := http.NewRequest(http.MethodGet, "/rest/v2/builds/build1/tasks"+query, nil)
		require.NoError(t, err)
		r = gimlet.SetURLVars(r, map[string]string{"build_id": buildID})

		tbh := makeFetchTasksByBuild("").(*tasksByBuildHandler).Factory()
		require.NoError(t, tbh.Parse(ctx, r))
		resp := tbh.Run(ctx)
		span.End()
		require.Equal(t, http.StatusOK, resp.Status())

		data, ok := resp.Data().([]any)
		require.True(t, ok)
		require.Len(t, data, 1)
		apiTask, ok := data[0].(*model.APITask)
		require.True(t, ok)
		require.Len(t, apiTask.Artifacts, 1)

		ended := spanRecorder.Ended()
		require.Len(t, ended, 1)
		attrs := map[attribute.Key]string{}
		for _, kv := range ended[0].Attributes() {
			attrs[kv.Key] = kv.Value.AsString()
		}
		return utility.FromStringPtr(apiTask.Artifacts[0].Link), attrs
	}

	t.Run("InvalidPresignValueShouldError", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/rest/v2/builds/build1/tasks?presign=eager", nil)
		require.NoError(t, err)
		r = gimlet.SetURLVars(r, map[string]string{"build_id": buildID})
		tbh := makeFetchTasksByBuild("").(*tasksByBuildHandler).Factory()
		assert.Error(t, tbh.Parse(t.Context(), r))
	})
	t.Run("NoPresignParamShouldPresignEagerly", func(t *testing.T) {
		settings.ArtifactSignSecret = testSecret
		link, attrs := runRequest(t, "")
		assert.Contains(t, link, "test-bucket")
		assert.NotContains(t, link, "/artifact/sign")
		assert.Equal(t, "eager", attrs[evergreen.ArtifactPresignRequestedModeOtelAttribute])
		assert.NotContains(t, attrs, attribute.Key(evergreen.ArtifactPresignModeOtelAttribute))
		assert.NotContains(t, attrs, attribute.Key(evergreen.ArtifactPresignFallbackReasonOtelAttribute))
	})
	t.Run("LazyPresignShouldReturnSignURL", func(t *testing.T) {
		settings.ArtifactSignSecret = testSecret
		link, attrs := runRequest(t, "?presign=lazy")
		assert.Contains(t, link, "/rest/v2/tasks/task1/artifact/sign?execution=0&name=signed_file&token=")
		assert.Equal(t, "lazy", attrs[evergreen.ArtifactPresignRequestedModeOtelAttribute])
		assert.Equal(t, "lazy", attrs[evergreen.ArtifactPresignModeOtelAttribute])
		assert.NotContains(t, attrs, attribute.Key(evergreen.ArtifactPresignFallbackReasonOtelAttribute))
	})
	t.Run("LazyPresignWithoutSecretShouldPresignEagerlyAndRecordFallback", func(t *testing.T) {
		settings.ArtifactSignSecret = ""
		link, attrs := runRequest(t, "?presign=lazy")
		assert.Contains(t, link, "test-bucket")
		assert.NotContains(t, link, "/artifact/sign")
		assert.Equal(t, "lazy", attrs[evergreen.ArtifactPresignRequestedModeOtelAttribute])
		assert.Equal(t, "eager", attrs[evergreen.ArtifactPresignModeOtelAttribute])
		assert.Equal(t, "no_sign_secret", attrs[evergreen.ArtifactPresignFallbackReasonOtelAttribute])
	})
}
