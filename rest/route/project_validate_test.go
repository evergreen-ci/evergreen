package route

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
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

	resp := handler.Run(t.Context())
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

func TestValidateProjectHandlerReportsParseWarnings(t *testing.T) {
	// A user-defined top-level _evg_anchors key parses fine but surfaces a
	// warning, since the key is reserved for Evergreen's internal use when
	// applying cross-file YAML anchors.
	yml := `
_evg_anchors:
  - &unused
    x: 1
tasks:
  - name: my_task
`

	require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	ref := model.ProjectRef{Id: "proj"}
	require.NoError(t, ref.Insert(t.Context()))

	input := validator.ValidationInput{
		ProjectYaml: []byte(yml),
		ProjectID:   "proj",
	}
	bodyBytes, err := json.Marshal(input)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/validate", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)

	handler := makeValidateProject().Factory()
	require.NoError(t, handler.Parse(t.Context(), req))

	resp := handler.Run(t.Context())
	require.NotNil(t, resp)

	rawBytes, err := json.Marshal(resp.Data())
	require.NoError(t, err)

	var results validator.ValidationErrors
	require.NoError(t, json.Unmarshal(rawBytes, &results))

	// CheckProject contributes its own warnings; assert that the parse warning
	// from the reserved key is surfaced among them.
	assert.True(t, slices.ContainsFunc(results, func(e validator.ValidationError) bool {
		return e.Level == validator.Warning && strings.Contains(e.Message, "top-level YAML key '_evg_anchors'")
	}), "expected the reserved _evg_anchors key warning in %v", results)
}
