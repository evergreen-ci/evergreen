package route

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-app-secret"

func TestArtifactSignHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(artifact.Collection))

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

	validToken, validExpiry := artifact.GenerateSignToken([]byte(testSecret), "task1", 0, "signed_report")

	t.Run("MissingTaskID", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks//artifact/sign?execution=0&name=signed_report&token=%s&exp=%d", validToken, validExpiry), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": ""})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("MissingExecution", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/task1/artifact/sign?name=signed_report&token=%s&exp=%d", validToken, validExpiry), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("MissingName", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/task1/artifact/sign?execution=0&token=%s&exp=%d", validToken, validExpiry), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("InvalidExecution", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/task1/artifact/sign?execution=-1&name=signed_report&token=%s&exp=%d", validToken, validExpiry), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("MissingToken", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/tasks/task1/artifact/sign?execution=0&name=signed_report", nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run("ExpiredToken", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/task1/artifact/sign?execution=0&name=signed_report&token=%s&exp=%d", validToken, 1000000000), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run("FileNotFound", func(t *testing.T) {
		token, expiry := artifact.GenerateSignToken([]byte(testSecret), "task1", 0, "nonexistent")
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/task1/artifact/sign?execution=0&name=nonexistent&token=%s&exp=%d", token, expiry), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
	t.Run("NotSignedFile", func(t *testing.T) {
		token, expiry := artifact.GenerateSignToken([]byte(testSecret), "task1", 0, "public_file")
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/task1/artifact/sign?execution=0&name=public_file&token=%s&exp=%d", token, expiry), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("SignedFileRedirects", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/task1/artifact/sign?execution=0&name=signed_report&token=%s&exp=%d", validToken, validExpiry), nil)
		req = gimlet.SetURLVars(req, map[string]string{"task_id": "task1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusTemporaryRedirect, rr.Code)
		location := rr.Header().Get("Location")
		assert.Contains(t, location, "test-bucket")
		assert.Contains(t, location, "path/to/file")
	})
}
