package operations

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen/validator"
	"github.com/urfave/cli"
)

func Validate() cli.Command {
	return cli.Command{
		Name:   "validate",
		Usage:  "verify that an evergreen project config is valid",
		Flag:   pathFlag(),
		Before: requirePathFlag,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			path := c.String(pathFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			_ = conf.GetRestCommunicator(ctx)

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			notifyUserUpdate(ac)

			confFile, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			projErrors, err := ac.ValidateLocalConfig(confFile)
			if err != nil {
				return nil
			}
			numErrors, numWarnings := 0, 0
			if len(projErrors) > 0 {
				for i, e := range projErrors {
					if e.Level == validator.Warning {
						numWarnings++
					} else if e.Level == validator.Error {
						numErrors++
					}
					fmt.Printf("%v) %v: %v\n\n", i+1, e.Level, e.Message)
				}

				return errors.Errorf("Project file has %d warnings, %d errors.", numWarnings, numErrors)
			}
			fmt.Println("Valid!")
			return nil

		},
	}
}
