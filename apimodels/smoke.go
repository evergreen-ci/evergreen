package apimodels

// APIBuild represents part of a build from the REST API for use in the smoke test.
type APIBuild struct {
	Tasks []string `json:"tasks"`
}

// APITask represents part of a task from the REST API for use in the smoke test.
type APITask struct {
	Status string            `json:"status"`
	Logs   map[string]string `json:"logs"`
}
