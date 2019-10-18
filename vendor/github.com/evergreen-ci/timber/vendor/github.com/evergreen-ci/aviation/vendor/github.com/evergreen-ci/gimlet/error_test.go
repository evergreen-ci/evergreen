package gimlet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorResponse(t *testing.T) {
	err := ErrorResponse{
		StatusCode: 418,
		Message:    "coffee",
	}
	out := err.Error()

	assert.Contains(t, out, "teapot")
	assert.Contains(t, out, "coffee")
	assert.Contains(t, out, "418")
}
