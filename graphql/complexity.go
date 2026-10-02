package graphql

import (
	"github.com/evergreen-ci/utility"
)

const (
	// testResultsFetchComplexity is the complexity cost of downloading a
	// task's full set of test results.
	testResultsFetchComplexity = 5
	defaultTestResultsCount    = 100
)

// setComplexityFuncs registers custom complexity calculations for fields
// whose cost is not reflected by gqlgen's default of one point per field.
func setComplexityFuncs(c *Config) {
	c.Complexity.Task.Tests = func(childComplexity int, opts *TestFilterOptions) int {
		if opts == nil {
			return testResultsFetchComplexity + childComplexity*defaultTestResultsCount
		}
		numResults := defaultTestResultsCount
		if limit := utility.FromIntPtr(opts.Limit); limit > 0 {
			numResults = limit
		}
		// Non-nil filter options mean the base task test results are also getting downloaded.
		return 2*testResultsFetchComplexity + childComplexity*numResults
	}
}
