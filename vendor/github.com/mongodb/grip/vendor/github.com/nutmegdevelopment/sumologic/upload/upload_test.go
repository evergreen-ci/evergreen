package upload // import "github.com/nutmegdevelopment/sumologic/upload"

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testData = []byte("Test message data")

func TestUploader(t *testing.T) {
	var data []byte
	var name string
	var contentEnc string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ = ioutil.ReadAll(r.Body)
		r.Body.Close()
		name = r.Header.Get("X-Sumo-Name")
		contentEnc = r.Header.Get("Content-Encoding")

		if len(data) == 0 {
			w.WriteHeader(400)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	// Invalid URLs should fail
	u := NewUploader("junk://")
	err := u.Send(testData, "")
	assert.Error(t, err)

	u = NewUploader(ts.URL)
	assert.Equal(t, ts.URL, u.(*httpUploader).url)

	// Basic usage
	err = u.Send(testData, "")
	assert.NoError(t, err)
	assert.Equal(t, testData, data)
	assert.Empty(t, name)
	assert.Empty(t, contentEnc)

	// nil input should not panic
	assert.NotPanics(t, func() {
		u.Send(nil, "")
	})

	// Empty data should return an error
	err = u.Send([]byte{}, "")
	assert.Error(t, err)

	// Test that name is sent
	err = u.Send(testData, "testname")
	assert.NoError(t, err)
	assert.Equal(t, testData, data)
	assert.Equal(t, "testname", name)
	assert.Empty(t, contentEnc)

	// Test compression
	GzipThreshold = 0
	u.Send(testData, "")
	assert.NoError(t, err)
	assert.Empty(t, name)
	assert.Equal(t, "gzip", contentEnc)

	r, err := gzip.NewReader(bytes.NewReader(data))
	assert.NoError(t, err)

	buf, err := ioutil.ReadAll(r)
	assert.NoError(t, err)
	assert.Equal(t, testData, buf)
}
