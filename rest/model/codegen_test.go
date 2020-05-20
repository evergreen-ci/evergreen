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
		"Author":          ExtractedField{OutputFieldName: "One", Nullable: true},
		"AuthorEmail":     ExtractedField{OutputFieldName: "Two", Nullable: false},
		"AuthorGithubUID": ExtractedField{OutputFieldName: "Three", Nullable: false},
	}
	generated, err := CreateConversionMethods("github.com/evergreen-ci/evergreen/model", "Revision", fields)
	assert.NoError(t, err)
	expected := `package model

import "github.com/evergreen-ci/evergreen/model"

func (m *APIRevision) BuildFromService(t model.Revision) error {
	m.One = stringToStringPtr(t.Author)
	m.Three = intToInt(t.AuthorGithubUID)
	m.Two = stringToString(t.AuthorEmail)
	return nil
}

func (m *APIRevision) ToService() (model.Revision, error) {
	out := model.Revision{}
	out.Author = stringToStringPtr(m.One)
	out.AuthorEmail = stringToString(m.Two)
	out.AuthorGithubUID = intToInt(m.Three)
	return out, nil
}
`
	assert.Equal(t, expected, string(generated))
}
