package apiv3

// APIError implements the Error() interface
type APIError struct {
	StatusCode int
	Message    string
}

func (e APIError) Error() string {
	return e.Message
}
