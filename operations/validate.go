package operations

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

func Validate() cli.Command {

	return cli.Command{
		Name:  "validate",
		Usage: "verify that an evergreen project config is valid",
		Flags: addPathFlag(cli.BoolFlag{
			Name:  joinFlagNames(quietFlagName, "q"),
			Usage: "suppress warnings",
		}, cli.BoolFlag{
			Name:  joinFlagNames(longFlagName, "l"),
			Usage: "include long validation checks (only applies if the check is over some threshold, in which case a warning is issued)",
		}, cli.StringSliceFlag{
			Name:  joinFlagNames(localModulesFlagName, "lm"),
			Usage: "specify local modules as MODULE_NAME=PATH pairs",
		}),
		Before: mergeBeforeFuncs(setPlainLogger, requirePathFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			path := c.String(pathFlagName)
			quiet := c.Bool(quietFlagName)
			long := c.Bool(longFlagName)
			localModulePaths := c.StringSlice(localModulesFlagName)
			localModuleMap, err := getLocalModulesFromInput(localModulePaths)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			fileInfo, err := os.Stat(path)
			if err != nil {
				return errors.Wrap(err, "problem getting file info")
			}

			if fileInfo.Mode()&os.ModeDir != 0 { // directory
				files, err := ioutil.ReadDir(path)
				if err != nil {
					return errors.Wrap(err, "problem reading directory")
				}
				catcher := grip.NewSimpleCatcher()
				for _, file := range files {
					catcher.Add(validateFile(filepath.Join(path, file.Name()), ac, quiet, long, localModuleMap))
				}
				return catcher.Resolve()
			}

			return validateFile(path, ac, quiet, long, localModuleMap)
		},
	}
}
func getLocalModulesFromInput(localModulePaths []string) (map[string]string, error) {
	moduleMap := make(map[string]string)
	catcher := grip.NewBasicCatcher()
	for _, module := range localModulePaths {
		pair := strings.Split(module, "=")
		if len(pair) != 2 {
			catcher.Errorf("expected only one '=' sign while parsing local module '%s'", module)
		} else {
			moduleMap[pair[0]] = pair[1]
		}
	}
	return moduleMap, catcher.Resolve()
}

func validateFile(path string, ac *legacyClient, quiet, includeLong bool, localModuleMap map[string]string) error {
	confFile, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrap(err, "problem reading file")
	}

	project := &model.Project{}
	ctx := context.Background()
	opts := &model.GetProjectOpts{
		LocalModules: localModuleMap,
		ReadFileFrom: model.ReadFromLocal,
	}
	pp, err := model.LoadProjectInto(ctx, confFile, opts, "", project)
	if err != nil {
		return errors.Wrapf(err, "%s is an invalid configuration", path)
	}
	projectYaml, err := yaml.Marshal(pp)
	if err != nil {
		return errors.Wrapf(err, "Could not marshal parser project into yaml")
	}

	projErrors, err := ac.ValidateLocalConfig(projectYaml, quiet, includeLong)
	if err != nil {
		return nil
	}

	if len(projErrors) != 0 {
		grip.Info(projErrors)
	}
	hasErrs := false
	hasWarnings := false
	for _, projErr := range projErrors {
		if projErr.Level == validator.Error {
			hasErrs = true
		} else if projErr.Level == validator.Warning {
			hasWarnings = true
		}
	}

	if hasErrs {
		return errors.Errorf("%s is an invalid configuration", path)
	} else if hasWarnings {
		grip.Infof("%s is valid with warnings", path)
	} else {
		grip.Infof("%s is valid", path)
	}

	return nil
}
