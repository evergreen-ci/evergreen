package service

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDockerfile(t *testing.T) {
	assert := assert.New(t)

	req, err := http.NewRequest("GET", "/hosts/dockerfile", nil)
	assert.NoError(err)
	w := httptest.NewRecorder()
	getDockerfile(w, req)

	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(err)

	parts := []string{
		"ARG BASE_IMAGE",
		"FROM $BASE_IMAGE",
		"ARG URL",
		"ARG EXECUTABLE_SUB_PATH",
		"ARG BINARY_NAME",
		"ADD ${URL}/clients/${EXECUTABLE_SUB_PATH} /root/",
		"RUN chmod +x /root/${BINARY_NAME}",
	}

	assert.Equal(strings.Join(parts, "\n"), string(body))
}
