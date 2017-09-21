package gimlet

import (
	"bytes"
	"io"
	"io/ioutil"
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

	for _, parserFunc := range []jsonishParser{GetYAML, GetJSON} {

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
	}
}
