package operations

import (
	"bytes"
	"context"
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
			Name:  joinFlagNames(errorOnWarningsFlagName, "w"),
			Usage: "treat warnings as errors",
		}, cli.StringSliceFlag{
			Name:  joinFlagNames(localModulesFlagName, "lm"),
			Usage: "specify local modules for included files as MODULE_NAME=PATH pairs",
		}, cli.StringFlag{
			Name:  joinFlagNames(projectFlagName, "p"),
			Usage: "specify project identifier in order to run validation requiring project settings",
		}),
		Before: mergeBeforeFuncs(autoUpdateCLI, setPlainLogger, requirePathFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(ConfFlagName)
			path := c.String(pathFlagName)
			quiet := c.Bool(quietFlagName)
			errorOnWarnings := c.Bool(errorOnWarningsFlagName)
			projectID := c.String(projectFlagName)
			localModulePaths := c.StringSlice(localModulesFlagName)
			localModuleMap, err := getLocalModulesFromInput(localModulePaths)
			if err != nil {
				return err
			}

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			if projectID == "" {
				cwd, err := os.Getwd()
				grip.Error(errors.Wrap(err, "getting current working directory"))
				cwd, err = filepath.EvalSymlinks(cwd)
				grip.Error(errors.Wrapf(err, "resolving symlinks for current working directory '%s'", cwd))
				projectID = conf.FindDefaultProject(cwd, false)
			}

			fileInfo, err := os.Stat(path)
			if err != nil {
				return errors.Wrapf(err, "getting file info for path '%s'", path)
			}

			if fileInfo.Mode()&os.ModeDir != 0 { // directory
				files, err := os.ReadDir(path)
				if err != nil {
					return errors.Wrapf(err, "reading directory '%s'", path)
				}
				catcher := grip.NewSimpleCatcher()
				for _, file := range files {
					catcher.Add(validateFile(conf, filepath.Join(path, file.Name()), quiet, errorOnWarnings, localModuleMap, projectID))
				}
				return catcher.Resolve()
			}

			return validateFile(conf, path, quiet, errorOnWarnings, localModuleMap, projectID)
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

func validateFile(conf *ClientSettings, path string, quiet, errorOnWarnings bool, localModuleMap map[string]string, projectID string) error {
	projectYaml, err := loadProjectYAML(path, quiet, errorOnWarnings, localModuleMap, projectID)
	if err != nil {
		return err
	}
	return validateProjectRemotely(conf, projectYaml, path, quiet, errorOnWarnings, projectID)
}

// loadProjectYAML reads and parses the project config file, performs local validation,
// and returns the marshalled YAML bytes for remote validation.
func loadProjectYAML(path string, quiet, errorOnWarnings bool, localModuleMap map[string]string, projectID string) ([]byte, error) {
	confFile, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "reading file '%s'", path)
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
	pp, pc, validationErrs := loadProjectIntoWithValidation(ctx, confFile, opts, errorOnWarnings, project, projectID)
	grip.Info(validationErrs)
	if validationErrs.Has(validator.Error) {
		return nil, errors.Errorf("%s is an invalid configuration", path)
	}

	projectYaml, err := yaml.Marshal(pp)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling parser project into YAML")
	}

	if pc != nil {
		projectConfigYaml, err := yaml.Marshal(pc.ProjectConfigFields)
		if err != nil {
			return nil, errors.Wrapf(err, "marshalling project config into YAML")
		}
		projectBytes := [][]byte{projectYaml, projectConfigYaml}
		projectYaml = bytes.Join(projectBytes, []byte("\n"))
	}

	return projectYaml, nil
}

// validateProjectRemotely sends the project YAML to the server for validation and reports results.
func validateProjectRemotely(conf *ClientSettings, projectYaml []byte, path string, quiet, errorOnWarnings bool, projectID string) error {
	ctx := context.Background()
	client, err := conf.setupRestCommunicator(ctx, false)
	if err != nil {
		return errors.Wrap(err, "setting up REST communicator")
	}
	defer client.Close()

	projErrors, err := client.Validate(ctx, projectYaml, quiet, projectID)
	if err != nil {
		return errors.Wrapf(err, "validating project '%s'", projectID)
	}

	grip.Info(projErrors)
	if projErrors.Has(validator.Error) || (errorOnWarnings && projErrors.Has(validator.Warning)) {
		return errors.Errorf("%s is an invalid configuration", path)
	} else if projErrors.Has(validator.Warning) {
		grip.Infof("%s is valid with warnings/notices", path)
	} else if projErrors.Has(validator.Notice) {
		grip.Infof("%s is valid with notices", path)
	} else {
		grip.Infof("%s is valid", path)
	}

	return nil
}

// loadProjectIntoWithValidation returns a warning (instead of an error) if there's an error with unmarshalling strictly
func loadProjectIntoWithValidation(ctx context.Context, data []byte, opts *model.GetProjectOpts, errorOnWarnings bool,
	project *model.Project, projectID string) (*model.ParserProject, *model.ProjectConfig, validator.ValidationErrors) {
	errs := validator.ValidationErrors{}
	// We validate the project config regardless if version control is disabled for the project
	// to ensure that the config will remain valid if it is turned on.
	pc, err := model.CreateProjectConfig(data, "")
	if err != nil {
		errs = append(errs, validator.ValidationError{
			Level:   validator.Error,
			Message: err.Error(),
		})
	}
	pp, err := model.LoadProjectInto(ctx, data, opts, projectID, project)
	if err != nil {
		// If the error came from unmarshalling strict, try it again without strict to verify if
		// it's a legitimate unmarshal error or just an error from strict (which should be a warning)
		if !errorOnWarnings && strings.Contains(err.Error(), util.UnmarshalStrictError) {
			opts.UnmarshalStrict = false
			pp, err2 := model.LoadProjectInto(ctx, data, opts, projectID, project)
			if err2 == nil {
				errs = append(errs, validator.ValidationError{
					Level:   validator.Warning,
					Message: errors.Wrap(err, "strict unmarshalling YAML").Error(),
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
