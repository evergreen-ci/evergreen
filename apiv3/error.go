package apiv3

// APIError implements the Error() interface
type APIError struct {
	StatusCode int    `json:"status"`
	Message    string `json:"error"`
}

func (e APIError) Error() string {
	return e.Message
}
