package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectTestsRequestJSON(t *testing.T) {
	t.Run("MarshalsProjectToCanonicalProjectID", func(t *testing.T) {
		b, err := json.Marshal(SelectTestsRequest{Project: "my-project"})
		require.NoError(t, err)
		assert.Contains(t, string(b), `"project_id":"my-project"`)
		assert.NotContains(t, string(b), `"project":`)
	})
	t.Run("UnmarshalsCanonicalProjectID", func(t *testing.T) {
		var req SelectTestsRequest
		require.NoError(t, json.Unmarshal([]byte(`{"project_id":"my-project"}`), &req))
		assert.Equal(t, "my-project", req.Project)
	})
	t.Run("PreservesOtherFields", func(t *testing.T) {
		var req SelectTestsRequest
		require.NoError(t, json.Unmarshal([]byte(`{"project_id":"p","requester":"patch","build_variant":"v","task_id":"t1","task_name":"t","tests":["a"],"strategies":["s"]}`), &req))
		assert.Equal(t, "p", req.Project)
		assert.Equal(t, "patch", req.Requester)
		assert.Equal(t, "v", req.BuildVariant)
		assert.Equal(t, "t1", req.TaskID)
		assert.Equal(t, "t", req.TaskName)
		assert.Equal(t, []string{"a"}, req.Tests)
		assert.Equal(t, []string{"s"}, req.Strategies)
	})
}
