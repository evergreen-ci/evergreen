package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProjectHandler(t *testing.T) {
	invalidYml := `
tasks:
  - name: my_task

buildvariants:
  - name: my_build_variant
    display_name: My Build Variant
    run_on:
    - "not_real"
    tasks:
    - name: my_task
`

	require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	ref := model.ProjectRef{Id: "proj"}
	require.NoError(t, ref.Insert(t.Context()))

	input := validator.ValidationInput{
		ProjectYaml: []byte(invalidYml),
		ProjectID:   "proj",
	}
	bodyBytes, err := json.Marshal(input)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/validate", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)

	handler := makeValidateProject().Factory()
	require.NoError(t, handler.Parse(t.Context(), req))

	resp := handler.Run(context.Background())
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	rawBytes, err := json.Marshal(resp.Data())
	require.NoError(t, err)

	// Parse the response body to make sure we get errors
	var results validator.ValidationErrors
	require.NoError(t, json.Unmarshal(rawBytes, &results))
	require.NotEmpty(t, results, "Expected validation errors for invalid project input")
	var messages string
	for _, validationErr := range results {
		messages += validationErr.Message
	}
	assert.Contains(t, messages, "buildvariant 'my_build_variant' references a nonexistent distro or container named 'not_real'")
}
