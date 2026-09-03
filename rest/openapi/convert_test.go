package openapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// swaggerSpec returns a minimal but realistic Swagger 2.0 spec, shaped like the
// one swaggo generates for Evergreen.
func swaggerSpec(t *testing.T, schemas map[string]any) []byte {
	spec := map[string]any{
		"swagger":  "2.0",
		"info":     map[string]any{"title": "Evergreen REST v2 API", "version": "OPENAPI_VERSION_PLACEHOLDER"},
		"host":     "OPENAPI_HOST_PLACEHOLDER",
		"basePath": "/rest/v2",
		"schemes":  []string{"https"},
		"paths": map[string]any{
			"/tasks/{task_id}": map[string]any{
				"get": map[string]any{
					"summary": "Fetch a task",
					"parameters": []any{map[string]any{
						"name": "task_id", "in": "path", "required": true, "type": "string",
					}},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OK",
							"schema":      map[string]any{"$ref": "#/definitions/model.APITask"},
						},
					},
				},
			},
		},
		"definitions": schemas,
	}
	out, err := json.Marshal(spec)
	require.NoError(t, err)
	return out
}

func defaultSchemas() map[string]any {
	return map[string]any{
		"model.APITask": map[string]any{
			"type":       "object",
			"properties": map[string]any{"task_id": map[string]any{"type": "string"}},
		},
	}
}

func TestConvertProducesOpenAPI3(t *testing.T) {
	converted, err := Convert(swaggerSpec(t, defaultSchemas()))
	require.NoError(t, err)

	var spec map[string]any
	require.NoError(t, json.Unmarshal(converted, &spec))

	version, ok := spec["openapi"].(string)
	require.True(t, ok, "converted spec should declare an OpenAPI version")
	assert.True(t, len(version) > 0 && version[0] == '3', "expected an OpenAPI 3 version, got '%s'", version)
	assert.NotContains(t, spec, "swagger", "converted spec should not retain the Swagger 2.0 version field")
}

func TestConvertPreservesPathsAndSchemas(t *testing.T) {
	converted, err := Convert(swaggerSpec(t, defaultSchemas()))
	require.NoError(t, err)

	var spec struct {
		Paths      map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(converted, &spec))

	assert.Contains(t, spec.Paths, "/tasks/{task_id}")
	assert.Contains(t, spec.Components.Schemas, "model.APITask")
}

// The host and basePath become a server URL in OpenAPI 3. The publish script
// substitutes the placeholders afterwards, so they must survive conversion
// intact and must not be read as OpenAPI 3 server variables.
func TestConvertMovesHostToServerAndKeepsPlaceholders(t *testing.T) {
	converted, err := Convert(swaggerSpec(t, defaultSchemas()))
	require.NoError(t, err)

	var out struct {
		Servers []struct {
			URL       string         `json:"url"`
			Variables map[string]any `json:"variables"`
		} `json:"servers"`
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	require.NoError(t, json.Unmarshal(converted, &out))

	require.Len(t, out.Servers, 1)
	assert.Equal(t, "https://OPENAPI_HOST_PLACEHOLDER/rest/v2", out.Servers[0].URL)
	assert.Empty(t, out.Servers[0].Variables, "placeholder should not be treated as a server variable")
	assert.Equal(t, "OPENAPI_VERSION_PLACEHOLDER", out.Info.Version)
}

func TestConvertCollidingSchemaNamesShouldError(t *testing.T) {
	schemas := defaultSchemas()
	schemas["route.Variant"] = map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
	}
	schemas["route.variant"] = map[string]any{
		"type":       "object",
		"properties": map[string]any{"id": map[string]any{"type": "string"}},
	}

	_, err := Convert(swaggerSpec(t, schemas))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "route.Variant and route.variant")
}

func TestConvertMalformedSpecShouldError(t *testing.T) {
	_, err := Convert([]byte("not json"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing Swagger 2.0 spec")
}
