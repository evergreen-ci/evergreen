package model

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodegen(t *testing.T) {
	files, err := ioutil.ReadDir(filepath.Join("testdata", "schema"))
	require.NoError(t, err)
	for _, info := range files {
		name := strings.Replace(info.Name(), ".graphql", "", -1)
		t.Run(name, func(t *testing.T) {
			f, err := ioutil.ReadFile(filepath.Join("testdata", "schema", name+".graphql"))
			require.NoError(t, err)
			generated, err := SchemaToGo(string(f))
			require.NoError(t, err)

			expected, err := ioutil.ReadFile(filepath.Join("testdata", "expected", name+".go"))
			require.NoError(t, err)
			assert.Equal(t, string(expected), string(generated))
		})
	}
}

func TestWords(t *testing.T) {
	s := "thisIsAFieldName"
	assert.Equal(t, words(s), []string{"this", "is", "a", "field", "name"})
}

func TestCreateConversionMethods(t *testing.T) {
	fields := ExtractedFields{
		"AnotherString": ExtractedField{OutputFieldName: "One", Nullable: true},
		"StringPtr":     ExtractedField{OutputFieldName: "Two", Nullable: false},
		"Int":           ExtractedField{OutputFieldName: "Three", Nullable: false},
		"IntPtr":        ExtractedField{OutputFieldName: "Four", Nullable: true},
	}
	generated, err := CreateConversionMethods("github.com/evergreen-ci/evergreen/rest/model", "TestStruct", fields)
	assert.NoError(t, err)
	expected := `package model

import "github.com/evergreen-ci/evergreen/rest/model"

func (m *APITestStruct) BuildFromService(t model.TestStruct) error {
	m.Four = intPtrToIntPtr(t.IntPtr)
	m.One = stringToStringPtr(t.AnotherString)
	m.Three = intToInt(t.Int)
	m.Two = stringPtrToString(t.StringPtr)
	return nil
}

func (m *APITestStruct) ToService() (model.TestStruct, error) {
	out := model.TestStruct{}
	out.AnotherString = stringToStringPtr(m.One)
	out.Int = intToInt(m.Three)
	out.IntPtr = intPtrToIntPtr(m.Four)
	out.StringPtr = stringPtrToString(m.Two)
	return out, nil
}
`
	assert.Equal(t, expected, string(generated))
}
