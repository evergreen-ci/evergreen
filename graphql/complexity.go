package graphql

// testResultsFetchComplexity is the complexity cost of downloading a task's
// full set of test results.
const testResultsFetchComplexity = 5

// setComplexityFuncs registers custom complexity calculations for fields
// whose cost is not reflected by gqlgen's default of one point per field.
func setComplexityFuncs(c *Config) {
	c.Complexity.Task.Tests = func(childComplexity int, opts *TestFilterOptions) int {
		if opts == nil {
			return testResultsFetchComplexity + childComplexity
		}
		// Non-nil filter options mean the base task test results are also getting downloaded.
		return 2*testResultsFetchComplexity + childComplexity
	}
}
