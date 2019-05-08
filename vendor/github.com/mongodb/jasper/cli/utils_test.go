package cli

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadInputValidJSON(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":"bar","bat":"baz","qux":[1,2,3,4,5]}`)
	output := struct {
		Foo string `json:"foo"`
		Bat string `json:"bat"`
		Qux []int  `json:"qux"`
	}{}
	require.NoError(t, readInput(input, &output))
	assert.Equal(t, "bar", output.Foo)
	assert.Equal(t, "baz", output.Bat)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, output.Qux)
}

func TestReadInputInvalidInput(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":}`)
	output := struct {
		Foo string `json:"foo"`
	}{}
	assert.Error(t, readInput(input, &output))
}

func TestReadInputInvalidOutput(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":"bar"}`)
	output := make(chan struct{})
	assert.Error(t, readInput(input, output))
}

func TestWriteOutput(t *testing.T) {
	input := struct {
		Foo string `json:"foo"`
		Bat string `json:"bat"`
		Qux []int  `json:"qux"`
	}{
		Foo: "bar",
		Bat: "baz",
		Qux: []int{1, 2, 3, 4, 5},
	}
	inputBuf := bytes.NewBufferString(`
	{
	"foo": "bar",
	"bat": "baz",
	"qux": [1 ,2, 3, 4, 5]
	}
	`)
	inputString := inputBuf.String()
	output := &bytes.Buffer{}
	require.NoError(t, writeOutput(output, input))
	assert.Equal(t, noWhitespace(inputString), noWhitespace(output.String()))
}

func TestWriteOutputInvalidInput(t *testing.T) {
	input := make(chan struct{})
	output := &bytes.Buffer{}
	assert.Error(t, writeOutput(output, input))
}

func TestWriteOutputInvalidOutput(t *testing.T) {
	input := bytes.NewBufferString(`{"foo":"bar"}`)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	output, err := ioutil.TempFile(filepath.Join(filepath.Dir(cwd), "build"), "write_output.txt")
	require.NoError(t, err)
	defer os.RemoveAll(output.Name())
	require.NoError(t, output.Close())
	assert.Error(t, writeOutput(output, input))
}
