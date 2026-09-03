package main

import (
	"flag"
	"os"

	"github.com/evergreen-ci/evergreen/rest/openapi"
	"github.com/pkg/errors"
)

// swagger-to-openapi converts the generated Swagger 2.0 REST v2 spec into an
// OpenAPI 3 spec, in place by default. It is run as part of the API doc build.
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

	swaggerSpec, err := os.ReadFile(input)
	if err != nil {
		return errors.Wrapf(err, "reading Swagger spec '%s'", input)
	}

	openapiSpec, err := openapi.Convert(swaggerSpec)
	if err != nil {
		return err
	}

	if err := os.WriteFile(output, openapiSpec, 0644); err != nil {
		return errors.Wrapf(err, "writing OpenAPI 3 spec '%s'", output)
	}

	return nil
}
