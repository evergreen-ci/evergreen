package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/jasper/metabuild/generator"
	"github.com/mongodb/jasper/metabuild/model"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
)

// Generate creates a cli.Command to use the Jasper metabuild system.
func Generate() cli.Command {
	return cli.Command{
		Name:  "generate",
		Usage: "Generate JSON evergreen configurations.",
		Subcommands: []cli.Command{
			generateGolang(),
			generateMake(),
		},
	}
}

const (
	jsonFormat = "json"
	yamlFormat = "yaml"
)

func generatedConfigFormatter(format string) (func(*shrub.Configuration) ([]byte, error), error) {
	switch strings.ToLower(format) {
	case jsonFormat:
		return func(conf *shrub.Configuration) ([]byte, error) {
			return json.MarshalIndent(conf, "", "\t")
		}, nil
	case yamlFormat:
		return func(conf *shrub.Configuration) ([]byte, error) {
			return yaml.Marshal(conf)
		}, nil
	}
	return nil, errors.Errorf("unrecognized format '%s'", format)
}

const (
	workingDirFlagName    = "working_dir"
	generatorFileFlagName = "generator_file"
	controlFileFlagName   = "control_file"
	outputFileFlagName    = "output_file"
	outputFormatFlagName  = "output_format"
)

func generateGolang() cli.Command {
	return cli.Command{
		Name:  "golang",
		Usage: "Generate JSON evergreen config from golang build file(s).",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  workingDirFlagName,
				Usage: "The directory that contains the GOPATH as a subdirectory.",
			},
			cli.StringFlag{
				Name:  generatorFileFlagName,
				Usage: "The build files necessary to generate the evergreen config.",
			},
			cli.StringFlag{
				Name:  controlFileFlagName,
				Usage: "The control file referencing all the necessary build files.",
			},
			cli.StringFlag{
				Name:  outputFileFlagName,
				Usage: "The output file. If unspecified, the config will be written to stdout.",
			},
			cli.StringFlag{
				Name:  outputFormatFlagName,
				Usage: "The output format (JSON or YAML).",
				Value: yamlFormat,
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(workingDirFlagName),
			requireOneFlag(generatorFileFlagName, controlFileFlagName),
			checkGeneratedConfigFormat,
		),
		Action: func(c *cli.Context) error {
			workingDir := c.String(workingDirFlagName)
			genFile := c.String(generatorFileFlagName)
			ctrlFile := c.String(controlFileFlagName)
			outputFile := c.String(outputFileFlagName)
			var err error
			if !filepath.IsAbs(workingDir) {
				workingDir, err = filepath.Abs(workingDir)
				if err != nil {
					return errors.Wrapf(err, "getting working directory '%s' as absolute path", workingDir)
				}
			}

			var g *model.Golang
			if genFile != "" {
				g, err = model.NewGolang(genFile, workingDir)
				if err != nil {
					return errors.Wrapf(err, "creating generator from build file '%s'", genFile)
				}
			} else if ctrlFile != "" {
				gc, err := model.NewGolangControl(ctrlFile, workingDir)
				if err != nil {
					return errors.Wrapf(err, "creating builder from control file '%s'", ctrlFile)
				}
				g, err = gc.Build()
				if err != nil {
					return errors.Wrapf(err, "creating model from control file '%s'", ctrlFile)
				}
			}

			gen := generator.NewGolang(*g)
			conf, err := gen.Generate()
			if err != nil {
				return errors.Wrap(err, "generating evergreen config from golang build file(s)")
			}

			if err := formatAndOutputGeneratedConfig(conf, c.String(outputFormatFlagName), outputFile); err != nil {
				return errors.Wrap(err, "formatting and writing output")
			}

			return nil
		},
	}
}

func generateMake() cli.Command {
	return cli.Command{
		Name:  "make",
		Usage: "Generate JSON evergreen config from make build file(s).",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  workingDirFlagName,
				Usage: "The directory containing the project and build files.",
			},
			cli.StringFlag{
				Name:  generatorFileFlagName,
				Usage: "The build files necessary to generate the evergreen config.",
			},
			cli.StringFlag{
				Name:  controlFileFlagName,
				Usage: "The control file referencing all the necessary build files.",
			},
			cli.StringFlag{
				Name:  outputFileFlagName,
				Usage: "The output file. If unspecified, the config will be written to stdout.",
			},
			cli.StringFlag{
				Name:  outputFormatFlagName,
				Usage: "The output format (JSON or YAML).",
				Value: yamlFormat,
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(workingDirFlagName),
			requireOneFlag(generatorFileFlagName, controlFileFlagName),
			checkGeneratedConfigFormat,
			cleanupFilePathSeparators(generatorFileFlagName, controlFileFlagName, workingDirFlagName),
		),
		Action: func(c *cli.Context) error {
			workingDir := c.String(workingDirFlagName)
			genFile := c.String(generatorFileFlagName)
			ctrlFile := c.String(controlFileFlagName)
			outputFile := c.String(outputFileFlagName)

			var m *model.Make
			var err error
			if genFile != "" {
				m, err = model.NewMake(genFile, workingDir)
				if err != nil {
					return errors.Wrapf(err, "creating model from build file '%s'", genFile)
				}
			} else if ctrlFile != "" {
				var mc *model.MakeControl
				mc, err = model.NewMakeControl(ctrlFile, workingDir)
				if err != nil {
					return errors.Wrapf(err, "creating builder from control file '%s'", ctrlFile)
				}
				m, err = mc.Build()
				if err != nil {
					return errors.Wrapf(err, "creating model from control file '%s'", ctrlFile)
				}
			}

			gen := generator.NewMake(*m)
			conf, err := gen.Generate()
			if err != nil {
				return errors.Wrapf(err, "generating evergreen config from build file(s)")
			}

			if err := formatAndOutputGeneratedConfig(conf, c.String(outputFormatFlagName), outputFile); err != nil {
				return errors.Wrap(err, "formatting and writing output")
			}

			return nil
		},
	}
}

func formatAndOutputGeneratedConfig(conf *shrub.Configuration, format, file string) error {
	doFormatting, err := generatedConfigFormatter(format)
	if err != nil {
		return errors.WithStack(err)
	}
	output, err := doFormatting(conf)
	if err != nil {
		return errors.Wrapf(err, "formatting configuration as '%s'", format)
	}

	if file == "" {
		fmt.Println(string(output))
		return nil
	}

	if err := ioutil.WriteFile(file, output, 0644); err != nil {
		return errors.Wrapf(err, "writing formatted config to file '%s'", file)
	}

	return nil
}

func checkGeneratedConfigFormat(c *cli.Context) error {
	format := c.String(outputFormatFlagName)
	if _, err := generatedConfigFormatter(format); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
