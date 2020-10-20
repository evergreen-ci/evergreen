package gimlet

import (
	"fmt"
	"net/http"
)

// APIError implements the Error() interface
type ErrorResponse struct {
	StatusCode int    `json:"status" yaml:"status"`
	Message    string `json:"error" yaml:"message"`
}

func (e ErrorResponse) Error() string {
	return fmt.Sprintf("%d (%s): %s", e.StatusCode, http.StatusText(e.StatusCode), e.Message)
}
