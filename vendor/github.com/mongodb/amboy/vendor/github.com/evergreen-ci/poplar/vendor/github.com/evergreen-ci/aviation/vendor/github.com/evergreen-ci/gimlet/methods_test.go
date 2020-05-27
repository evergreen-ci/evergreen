package gimlet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMethodString(t *testing.T) {
	assert := assert.New(t)
	cases := map[httpMethod]string{
		get:             "GET",
		put:             "PUT",
		delete:          "DELETE",
		patch:           "PATCH",
		post:            "POST",
		head:            "HEAD",
		httpMethod(100): "",
		httpMethod(50):  "",
	}

	for meth, out := range cases {
		assert.Equal(out, meth.String())
	}
}

func TestOutputFormat(t *testing.T) {
	assert := assert.New(t)
	cases := map[OutputFormat]string{
		JSON:   "json",
		TEXT:   "text",
		HTML:   "html",
		YAML:   "yaml",
		BINARY: "binary",
	}

	for meth, out := range cases {
		assert.Equal(out, meth.String())
		assert.True(meth.IsValid())
	}

	cases = map[OutputFormat]string{
		OutputFormat(500): "text",
		OutputFormat(50):  "text",
	}
	for meth, out := range cases {
		assert.Equal(out, meth.String())
		assert.False(meth.IsValid())
	}

	// content-type should default to text
	assert.Equal(OutputFormat(100).ContentType(),
		TEXT.ContentType())
}
