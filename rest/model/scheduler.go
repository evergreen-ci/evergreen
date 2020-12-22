package model

type CompareTasksRequest struct {
	Tasks []string `json:"tasks"`
}

type CompareTasksResponse struct {
	Order []string                     `json:"order"`
	Logic map[string]map[string]string `json:"logic"`
}
