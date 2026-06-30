package route

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactSignHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(artifact.Collection, user.Collection))

	entry := artifact.Entry{
		TaskId:    "task1",
		BuildId:   "build1",
		Execution: 0,
		Files: []artifact.File{
			{
				Name:       "signed_report",
				Link:       "https://bucket.s3.amazonaws.com/path/to/file",
				Visibility: artifact.Signed,
				Bucket:     "test-bucket",
				FileKey:    "path/to/file",
				AWSKey:     "key",
				AWSSecret:  "secret",
			},
			{
				Name:       "public_file",
				Link:       "https://example.com/public",
				Visibility: artifact.Public,
			},
		},
	}
	require.NoError(t, entry.Upsert(t.Context()))

	handler := artifactSignHandler()

	t.Run("MissingTaskID", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/tasks//artifact/sign?execution=0&name=signed_report", nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": ""})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("MissingExecution", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/tasks/task1/artifact/sign?name=signed_report", nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("MissingName", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/tasks/task1/artifact/sign?execution=0", nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("InvalidExecution", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/tasks/task1/artifact/sign?execution=-1&name=signed_report", nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("FileNotFound", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/tasks/task1/artifact/sign?execution=0&name=nonexistent", nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
	t.Run("NotSignedFile", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/tasks/task1/artifact/sign?execution=0&name=public_file", nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
