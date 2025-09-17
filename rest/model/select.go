package model

// SelectTestsRequest represents a request to return a filtered set of tests to
// run. It deliberately includes information that could be looked up in the
// database in order to bypass database lookups. This allows Evergreen to pass
// this information directly to the test selector.
type SelectTestsRequest struct {
	// Project is the project identifier.
	Project string `json:"project"`
	// Requester is the Evergreen requester type.
	Requester string `json:"requester"`
	// BuildVariant is the Evergreen build variant.
	BuildVariant string `json:"build_variant"`
	// TaskID is the Evergreen task ID.
	TaskID string `json:"task_id"`
	// TaskName is the Evergreen task name.
	TaskName string `json:"task_name"`
	// Tests is a list of test names.
	Tests []string `json:"tests"`
	// Strategies is the optional list of test selection strategies to use.
	Strategies []string `json:"strategies"`
}
