package graphql

import (
	"github.com/evergreen-ci/utility"
)

const (
	// testResultsFetchComplexity is the complexity cost of downloading a
	// task's full set of test results.
	testResultsFetchComplexity = 5
	unboundedTestResultsCount  = 100
)

// setComplexityFuncs registers custom complexity calculations for fields
// whose cost is not reflected by gqlgen's default of one point per field.
func setComplexityFuncs(c *Config) {
	c.Complexity.Task.Tests = func(childComplexity int, opts *TestFilterOptions) int {
		fetches := 1
		numResults := unboundedTestResultsCount
		if opts != nil {
			// Non-nil filter options mean the base task test results are also getting downloaded.
			fetches = 2
			if limit := utility.FromIntPtr(opts.Limit); limit > 0 {
				numResults = limit
			}
		}
		return fetches*testResultsFetchComplexity + childComplexity*numResults
	}
}
