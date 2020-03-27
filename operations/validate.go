package operations

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Validate() cli.Command {
	const (
		quietFlagName = "quiet"
	)
	return cli.Command{
		Name:  "validate",
		Usage: "verify that an evergreen project config is valid",
		Flags: addPathFlag(cli.BoolFlag{
			Name:  joinFlagNames(quietFlagName, "q"),
			Usage: "suppress warnings",
		},
		),
		Before: mergeBeforeFuncs(setPlainLogger, requirePathFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			path := c.String(pathFlagName)
			quiet := c.Bool(quietFlagName)
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
					catcher.Add(validateFile(filepath.Join(path, file.Name()), ac, quiet))
				}
				return catcher.Resolve()
			}

			return validateFile(path, ac, quiet)
		},
	}
}

func validateFile(path string, ac *legacyClient, quiet bool) error {
	confFile, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrap(err, "problem reading file")
	}

	projErrors, err := ac.ValidateLocalConfig(confFile)
	if err != nil {
		return nil
	}

	if quiet { // only return errors
		allErrors := projErrors
		projErrors = []validator.ValidationError{}
		for _, e := range allErrors {
			if e.Level == validator.Error {
				projErrors = append(projErrors, e)
			}
		}
	}
	if len(projErrors) == 0 {
		grip.Infof("%s is valid", path)
	} else {
		grip.Info(projErrors)
	}

	for _, e := range projErrors {
		if e.Level == validator.Error {
			return errors.New("invalid configuration for %s")
		}
	}

	return nil
}
