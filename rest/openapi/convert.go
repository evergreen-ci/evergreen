// Package openapi converts Evergreen's generated REST v2 API spec from Swagger
// 2.0 to OpenAPI 3.
//
// swaggo, which generates the spec from code comments, only emits Swagger 2.0,
// so the spec is converted as a post-processing step of the API doc build.
package openapi

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pkg/errors"
)

// Convert converts a Swagger 2.0 spec into an indented OpenAPI 3 spec. It
// validates the result, so a spec that fails to convert cleanly is reported
// here rather than published to users.
func Convert(swaggerSpec []byte) ([]byte, error) {
	var swagger openapi2.T
	if err := json.Unmarshal(swaggerSpec, &swagger); err != nil {
		return nil, errors.Wrap(err, "parsing Swagger 2.0 spec")
	}

	openapiSpec, err := openapi2conv.ToV3(&swagger)
	if err != nil {
		return nil, errors.Wrap(err, "converting Swagger 2.0 spec to OpenAPI 3")
	}

	if err := validate(openapiSpec); err != nil {
		return nil, err
	}

	converted, err := json.MarshalIndent(openapiSpec, "", "    ")
	if err != nil {
		return nil, errors.Wrap(err, "marshalling OpenAPI 3 spec")
	}

	return append(converted, '\n'), nil
}

func validate(spec *openapi3.T) error {
	// Validation needs its own loader context rather than a request context,
	// since it only resolves references within the spec itself.
	loader := openapi3.NewLoader()
	if err := spec.Validate(loader.Context); err != nil {
		return errors.Wrap(err, "validating converted OpenAPI 3 spec")
	}

	return checkSchemaNameCollisions(spec)
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]`)

// checkSchemaNameCollisions reports schema names that differ only by case or
// punctuation. Such names are legal OpenAPI, but client generators normalize
// them to a single language-idiomatic class name and silently drop the
// colliding schemas along with the endpoints that use them. See DEVPROD-42404.
func checkSchemaNameCollisions(spec *openapi3.T) error {
	if spec.Components == nil {
		return nil
	}

	normalizedToNames := map[string][]string{}
	for name := range spec.Components.Schemas {
		normalized := nonAlphanumeric.ReplaceAllString(strings.ToLower(name), "")
		normalizedToNames[normalized] = append(normalizedToNames[normalized], name)
	}

	var collisions []string
	for _, names := range normalizedToNames {
		if len(names) > 1 {
			slices.Sort(names)
			collisions = append(collisions, strings.Join(names, " and "))
		}
	}
	if len(collisions) == 0 {
		return nil
	}
	slices.Sort(collisions)

	return errors.Errorf("schema names collide once normalized, which breaks client generation; rename one of each of the following: %s", strings.Join(collisions, "; "))
}
