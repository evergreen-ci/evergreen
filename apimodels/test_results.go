package apimodels

// DisplayTaskInfo represents information about a display task necessary for
// creating a cedar test result.
type DisplayTaskInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
