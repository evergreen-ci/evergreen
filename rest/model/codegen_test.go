package model

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestCodegen(t *testing.T) {
	configFile, err := ioutil.ReadFile("testdata/schema/config.yml")
	require.NoError(t, err)
	var gqlConfig config.Config
	err = yaml.Unmarshal(configFile, &gqlConfig)
	require.NoError(t, err)
	mapping := ModelMapping{}
	for dbModel, info := range gqlConfig.Models {
		mapping[dbModel] = info.Model[0]
	}

	schemaFiles, err := ioutil.ReadDir(filepath.Join("testdata", "schema"))
	require.NoError(t, err)
	for _, info := range schemaFiles {
		if !strings.HasSuffix(info.Name(), ".graphql") {
			continue
		}
		name := strings.Replace(info.Name(), ".graphql", "", -1)
		t.Run(name, func(t *testing.T) {
			f, err := ioutil.ReadFile(filepath.Join("testdata", "schema", name+".graphql"))
			require.NoError(t, err)
			generated, err := Codegen(string(f), mapping)
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

func TestcreateConversionMethods(t *testing.T) {
	fields := extractedFields{
		"Author":          extractedField{OutputFieldName: "One", OutputFieldType: "*string", Nullable: true},
		"AuthorEmail":     extractedField{OutputFieldName: "Two", OutputFieldType: "string", Nullable: false},
		"AuthorGithubUID": extractedField{OutputFieldName: "Three", OutputFieldType: "int", Nullable: false},
	}
	generated, err := createConversionMethods("github.com/evergreen-ci/evergreen/model", "Revision", fields)
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

func TestcreateConversionMethodsError(t *testing.T) {
	fields := extractedFields{
		"Author": extractedField{OutputFieldName: "One", OutputFieldType: "int", Nullable: true},
	}
	_, err := createConversionMethods("github.com/evergreen-ci/evergreen/model", "Revision", fields)
	assert.EqualError(t, err, `DB model field 'Author' has type string which is incompatible with REST model type int`)
}
