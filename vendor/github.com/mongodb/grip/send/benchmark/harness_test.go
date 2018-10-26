package send

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func BenchmarkAllSenders(b *testing.B) {
	outputFileName := getProjectRoot() + string(os.PathSeparator) + "build" + string(os.PathSeparator) + "perf.json"

	ctx := context.Background()
	output := []interface{}{}
	for _, res := range runAllCases(ctx) {
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

	return
}

func runAllCases(ctx context.Context) []*benchResult {
	cases := getAllCases()

	results := []*benchResult{}
	for _, bc := range cases {
		results = append(results, bc.Run(ctx))
	}

	return results
}
