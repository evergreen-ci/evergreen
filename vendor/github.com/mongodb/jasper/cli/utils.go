package cli

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// mergeBeforeFuncs returns a cli.BeforeFunc that runs all funcs and accumulates
// the errors.
func mergeBeforeFuncs(funcs ...cli.BeforeFunc) cli.BeforeFunc {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()
		for _, f := range funcs {
			catcher.Add(f(c))
		}
		return catcher.Resolve()
	}
}

// joinFlagNames joins multiple CLI flag names.
func joinFlagNames(names ...string) string {
	return strings.Join(names, ", ")
}

// readInput reads JSON from the input and decodes it to the output.
func readInput(input io.Reader, output interface{}) error {
	bytes, err := ioutil.ReadAll(input)
	if err != nil {
		return errors.Wrap(err, "error reading from input")
	}
	return errors.Wrap(json.Unmarshal(bytes, output), "error decoding to output")
}

// writeOutput encodes the output as JSON and writes it to w.
func writeOutput(output io.Writer, input interface{}) error {
	bytes, err := json.MarshalIndent(input, "", "    ")
	if err != nil {
		return errors.Wrap(err, "error encoding input")
	}
	if _, err := output.Write(bytes); err != nil {
		return errors.Wrap(err, "error writing to output")
	}

	return nil
}
