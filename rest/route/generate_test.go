package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/stretchr/testify/assert"
)

type Thing struct {
	Thing string `json:"thing"`
}

func TestValidateJSON(t *testing.T) {
	assert := assert.New(t)
	jsonBytes := []byte(`
[
{
  "thing": "one"
},
{
  "thing": "two"
}
]
`)
	buffer := bytes.NewBuffer(jsonBytes)
	request, err := http.NewRequest("", "", buffer)
	assert.NoError(err)
	files, err := parseJson(request)
	var thing Thing
	for i, f := range files {
		assert.NoError(json.Unmarshal(f, &thing))
		switch i {
		case 0:
			assert.Equal("one", thing.Thing)
		case 1:
			assert.Equal("two", thing.Thing)
		}
	}
}

func TestGenerateExecute(t *testing.T) {
	assert := assert.New(t)
	h := &generateHandler{}
	r, err := h.Execute(context.Background(), &data.MockConnector{})
	assert.Equal(ResponseData{}, r)
	assert.NoError(err)
}
