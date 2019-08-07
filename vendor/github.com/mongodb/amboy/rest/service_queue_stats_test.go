// +build go1.7

package rest

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusOuputGenerator(t *testing.T) {
	assert := assert.New(t)
	service := NewQueueService()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := service.getStatus(ctx)
	assert.Equal("degraded", st.Status)
	assert.False(st.QueueRunning)
	assert.True(len(st.SupportedJobTypes) > 0)

	// Now open the the service, and thus the queue, and watch the response change:
	assert.NoError(service.Open(ctx))
	defer service.Close()

	st = service.getStatus(ctx)
	assert.Equal("ok", st.Status)
	assert.True(st.QueueRunning)
	assert.True(len(st.SupportedJobTypes) > 0)
}

func TestStatusMethod(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx := context.Background()
	service := NewQueueService()

	w := httptest.NewRecorder()

	service.Status(w, httptest.NewRequest("GET", "http://example.com/status", nil))
	assert.Equal(200, w.Code)

	st := status{}
	assert.NoError(json.Unmarshal(w.Body.Bytes(), &st))
	assert.Equal("degraded", st.Status)
	assert.False(st.QueueRunning)
	assert.True(len(st.SupportedJobTypes) > 0)

	// Now open the the service, and thus the queue, and watch the response change:
	assert.NoError(service.Open(ctx))
	defer service.Close()

	w = httptest.NewRecorder()

	service.Status(w, httptest.NewRequest("GET", "http://example.com/status", nil))
	assert.Equal(200, w.Code)

	st = status{}
	assert.NoError(json.Unmarshal(w.Body.Bytes(), &st))
	assert.Equal("ok", st.Status)
	assert.True(st.QueueRunning)
	assert.True(len(st.SupportedJobTypes) > 0)
}
