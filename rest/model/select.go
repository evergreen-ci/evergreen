package model

import "encoding/json"

// SelectTestsRequest represents a request to return a filtered set of tests to
// run. It deliberately includes information that could be looked up in the
// database in order to bypass database lookups. This allows Evergreen to pass
// this information directly to the test selector.
type SelectTestsRequest struct {
	// Project is the project identifier.
	Project string `json:"project_id"`
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

// UnmarshalJSON accepts the legacy "project" field name in addition to the
// canonical "project_id" so the endpoint keeps working for clients that have
// not yet adopted "project_id" (e.g. agents that haven't rolled forward).
// "project_id" takes precedence when both are present.
func (r *SelectTestsRequest) UnmarshalJSON(data []byte) error {
	type alias SelectTestsRequest
	aux := struct {
		*alias
		LegacyProject string `json:"project"`
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if r.Project == "" {
		r.Project = aux.LegacyProject
	}
	return nil
}
