package gimlet

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type jsonishParser func(io.ReadCloser, interface{}) error

func TestReadingStructuredDataFromRequestBodies(t *testing.T) {
	assert := assert.New(t)

	type form struct {
		Name  string `json:"name" yaml:"name"`
		Count int    `json:"count" yaml:"count"`
	}

	for _, parserFunc := range []jsonishParser{GetYAML, GetJSON, GetJSONUnlimited, GetYAMLUnlimited} {

		// case one: everything works

		buf := bytes.NewBufferString(`{"name": "gimlet", "count": 2}`)

		bufcloser := ioutil.NopCloser(buf)
		out := form{}

		assert.NoError(parserFunc(bufcloser, &out))
		assert.Equal(2, out.Count)
		assert.Equal("gimlet", out.Name)

		// case two: basd json

		buf = bytes.NewBufferString(`{"name": "gimlet"`)
		bufcloser = ioutil.NopCloser(buf)
		out2 := form{}

		assert.Error(parserFunc(bufcloser, &out2))

		assert.Zero(out2.Count)
		assert.Zero(out2.Name)

		assert.Error(parserFunc(nil, map[string]string{}))
	}
}

func TestGetVarsInvocation(t *testing.T) {
	assert.Panics(t, func() { GetVars(nil) })
	assert.Nil(t, GetVars(&http.Request{}))
	assert.Nil(t, GetVars(httptest.NewRequest("GET", "http://localhost/bar", nil)))
}

type erroringReadCloser struct{}

func (*erroringReadCloser) Read(_ []byte) (int, error) { return 0, errors.New("test") }
func (*erroringReadCloser) Close() error               { return nil }

func TestRequestReadingErrorPropogating(t *testing.T) {
	errc := &erroringReadCloser{}

	assert.Error(t, GetJSON(errc, "foo"))
	assert.Error(t, GetYAML(errc, "foo"))
	assert.Error(t, GetJSONUnlimited(errc, "foo"))
	assert.Error(t, GetYAMLUnlimited(errc, "foo"))
}
