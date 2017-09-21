// +build go1.7

package rest

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestStatusOuputGenerator(t *testing.T) {
	assert := assert.New(t)
	service := NewService()
	ctx := context.Background()

	st := service.getStatus()
	assert.Equal("degraded", st.Status)
	assert.False(st.QueueRunning)
	assert.True(len(st.SupportedJobTypes) > 0)

	// Now open the the service, and thus the queue, and watch the response change:
	assert.NoError(service.Open(ctx))
	defer service.Close()

	st = service.getStatus()
	assert.Equal("ok", st.Status)
	assert.True(st.QueueRunning)
	assert.True(len(st.SupportedJobTypes) > 0)
}

func TestStatusMethod(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	service := NewService()

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
