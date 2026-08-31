package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/pkg/errors"
)

// swagger-to-openapi converts a Swagger 2.0 spec into an OpenAPI 3 spec.
// swaggo, which generates Evergreen's REST v2 spec from code comments, only
// emits Swagger 2.0, so the generated spec is converted as a post-processing
// step before it is rendered and published.
func main() {
	var input, output string
	flag.StringVar(&input, "input", "", "path to the Swagger 2.0 spec to convert")
	flag.StringVar(&output, "output", "", "path to write the OpenAPI 3 spec to; defaults to the input path")
	flag.Parse()

	if err := run(input, output); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(input, output string) error {
	if input == "" {
		return errors.New("input path must be specified")
	}
	if output == "" {
		output = input
	}

	contents, err := os.ReadFile(input)
	if err != nil {
		return errors.Wrapf(err, "reading Swagger spec '%s'", input)
	}

	var swagger openapi2.T
	if err := json.Unmarshal(contents, &swagger); err != nil {
		return errors.Wrapf(err, "parsing Swagger spec '%s'", input)
	}

	openapi, err := openapi2conv.ToV3(&swagger)
	if err != nil {
		return errors.Wrap(err, "converting Swagger 2.0 spec to OpenAPI 3")
	}

	converted, err := json.MarshalIndent(openapi, "", "    ")
	if err != nil {
		return errors.Wrap(err, "marshalling OpenAPI 3 spec")
	}

	if err := os.WriteFile(output, append(converted, '\n'), 0644); err != nil {
		return errors.Wrapf(err, "writing OpenAPI 3 spec '%s'", output)
	}

	return nil
}
