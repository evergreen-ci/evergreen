package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/util"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
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
		}, cli.StringFlag{
			Name:  joinFlagNames(projectFlagName, "p"),
			Usage: "specify project identifier in order to run validation requiring project settings",
		}),
		Before: mergeBeforeFuncs(setPlainLogger, requirePathFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			path := c.String(pathFlagName)
			quiet := c.Bool(quietFlagName)
			long := c.Bool(longFlagName)
			projectID := c.String(projectFlagName)
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

			if projectID == "" {
				cwd, err := os.Getwd()
				grip.Error(errors.Wrap(err, "unable to get current working directory"))
				cwd, err = filepath.EvalSymlinks(cwd)
				grip.Error(errors.Wrap(err, "unable to resolve symlinks"))
				projectID = conf.FindDefaultProject(cwd, false)
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
					catcher.Add(validateFile(filepath.Join(path, file.Name()), ac, quiet, long, localModuleMap, projectID))
				}
				return catcher.Resolve()
			}

			return validateFile(path, ac, quiet, long, localModuleMap, projectID)
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

func validateFile(path string, ac *legacyClient, quiet, includeLong bool, localModuleMap map[string]string, projectID string) error {
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
	if !quiet {
		opts.UnmarshalStrict = true
	}
	pp, pc, validationErrs := loadProjectIntoWithValidation(ctx, confFile, opts, project)
	grip.Info(validationErrs)
	if validationErrs.HasError() {
		return errors.Errorf("%s is an invalid configuration", path)
	}

	projectYaml, err := yaml.Marshal(pp)
	if err != nil {
		return errors.Wrapf(err, "Could not marshal parser project into yaml")
	}

	if pc != nil {
		headlessPc := model.GetHeadlessProjectConfig(pc)
		projectConfigYaml, err := yaml.Marshal(headlessPc)
		if err != nil {
			return errors.Wrapf(err, "Could not marshal project config into yaml")
		}
		projectYaml = []byte(string(projectYaml) + string(projectConfigYaml))
	}
	projErrors, err := ac.ValidateLocalConfig(projectYaml, quiet, includeLong, projectID)
	if err != nil {
		return nil
	}

	grip.Info(projErrors)
	if projErrors.HasError() {
		return errors.Errorf("%s is an invalid configuration", path)
	} else if len(projErrors) > 0 {
		grip.Infof("%s is valid with warnings", path)
	} else {
		grip.Infof("%s is valid", path)
	}

	return nil
}

// loadProjectIntoWithValidation returns a warning (instead of an error) if there's an error with unmarshalling strictly
func loadProjectIntoWithValidation(ctx context.Context, data []byte, opts *model.GetProjectOpts,
	project *model.Project) (*model.ParserProject, *model.ProjectConfig, validator.ValidationErrors) {
	errs := validator.ValidationErrors{}
	pp, pc, err := model.LoadProjectInto(ctx, data, opts, "", project)
	if err != nil {
		// If the error came from unmarshalling strict, try it again without strict to verify if
		// it's a legitimate unmarshal error or just an error from strict (which should be a warning)
		if strings.Contains(err.Error(), util.UnmarshalStrictError) {
			opts.UnmarshalStrict = false
			pp, _, err2 := model.LoadProjectInto(ctx, data, opts, "", project)
			if err2 == nil {
				errs = append(errs, validator.ValidationError{
					Level:   validator.Warning,
					Message: fmt.Sprintf("error unmarshalling strictly: %s", err.Error()),
				})
				return pp, pc, errs
			}
		}
		errs = append(errs, validator.ValidationError{
			Level:   validator.Error,
			Message: err.Error(),
		})
	}
	return pp, pc, errs
}
