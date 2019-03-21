package jasper

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func BenchmarkLoggers(b *testing.B) {
	outputFileName := "build/perf.json"

	ctx := context.Background()
	output := []interface{}{}
	for _, res := range runCases(ctx) {
		evg, err := res.evergreenPerfFormat()
		if err != nil {
			continue
		}

		output = append(output, evg...)
	}

	evgOutput, err := json.MarshalIndent(map[string]interface{}{"results": output}, "", "   ")
	if err != nil {
		return
	}
	evgOutput = append(evgOutput, []byte("\n")...)

	if outputFileName == "" {
		fmt.Println(string(evgOutput))
	} else if err := ioutil.WriteFile(outputFileName, evgOutput, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "problem writing file '%s': %s", outputFileName, err.Error())
		return
	}
}

func runCases(ctx context.Context) []*benchResult {
	cases := getAllCases()

	results := []*benchResult{}
	for _, bc := range cases {
		results = append(results, bc.run(ctx))
	}

	return results
}
