package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAwsSnsRun(t *testing.T) {
	aws := awsSns{messageType: "unknown"}
	responder := aws.Run(context.Background())
	assert.Equal(t, http.StatusBadRequest, responder.Status())
}
