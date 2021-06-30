package main

import (
	"flag"
	"io/ioutil"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/evergreen-ci/evergreen/cmd/codegen/core"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		configFile string
		schemaFile string
		modelFile  string
		helperFile string
	)
	flag.StringVar(&configFile, "config", "", "path to the config yml file with struct mappings, either an absolute path or relative to the evergreen folder")
	flag.StringVar(&schemaFile, "schema", "", "path to the graphql schema to use for the output, either an absolute path or relative to the evergreen folder")
	flag.StringVar(&modelFile, "model", "", "file path to write the generated model code, either an absolute path or relative to the evergreen folder")
	flag.StringVar(&helperFile, "helper", "", "file path to write the generated helper functions, either an absolute path or relative to the evergreen folder")
	flag.Parse()
	const pathToRestModels = "cmd/codegen/core"

	configData, err := ioutil.ReadFile(configFile)
	grip.EmergencyFatal(errors.Wrap(err, "unable to read config file"))
	var gqlConfig config.Config
	err = yaml.Unmarshal(configData, &gqlConfig)
	grip.EmergencyFatal(errors.Wrap(err, "unable to parse config file"))
	schema, err := ioutil.ReadFile(schemaFile)
	grip.EmergencyFatal(errors.Wrap(err, "unable to read schema file"))

	mapping := core.ModelMapping{}
	for dbModel, info := range gqlConfig.Models {
		mapping[dbModel] = info.Model[0]
	}
	core.SetGeneratePathPrefix(pathToRestModels)
	model, helper, err := core.Codegen(string(schema), mapping)
	grip.EmergencyFatal(errors.Wrap(err, "error generating code"))
	err = ioutil.WriteFile(modelFile, model, 0644)
	grip.EmergencyFatal(errors.Wrap(err, "error writing to model file"))
	err = ioutil.WriteFile(helperFile, helper, 0644)
	grip.EmergencyFatal(errors.Wrap(err, "error writing to helper file"))

	grip.Infof("%s and %s have been updated with the generated code", modelFile, helperFile)
}
