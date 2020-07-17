package model

import (
	"io/ioutil"
	"os"
	"os/exec"
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
			generated, converters, err := Codegen(string(f), mapping)
			require.NoError(t, err)

			expected, err := ioutil.ReadFile(filepath.Join("testdata", "expected", name+".go"))
			require.NoError(t, err)
			assert.Equal(t, string(expected), string(generated))

			converterFilepath := filepath.Join("testdata", "expected", name+"__converters.go")
			expected, err = ioutil.ReadFile(converterFilepath)
			require.NoError(t, err)
			assert.Equal(t, string(expected), string(converters))

			gocmd := os.Getenv("GO_BIN_PATH")
			if gocmd == "" {
				gocmd = "go"
			}
			buildCmd := exec.Command(gocmd, "build", converterFilepath)
			_, err = buildCmd.Output()
			assert.NoError(t, err, "%s does not compile", converterFilepath)
		})
	}
}

func TestWords(t *testing.T) {
	s := "thisIsAFieldName"
	assert.Equal(t, words(s), []string{"this", "is", "a", "field", "name"})
}
