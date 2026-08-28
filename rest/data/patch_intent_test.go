package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextForQueuedJobPreservesValuesAfterRequestCancellation(t *testing.T) {
	type contextKey string
	const key contextKey = "key"

	requestCtx, cancel := context.WithCancel(context.WithValue(t.Context(), key, "value"))
	queueCtx := contextForQueuedJob(requestCtx)
	cancel()

	assert.NoError(t, queueCtx.Err())
	assert.Equal(t, "value", queueCtx.Value(key))
}
