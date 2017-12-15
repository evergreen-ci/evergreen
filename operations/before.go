package operations

import (
	"errors"
	"os"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/urfave/cli"
)

var (
	requireClientConfig = func(c *cli.Context) error {
		if c.Parent().String(confFlagName) == "" {
			return errors.New("command line configuration path is not specified")
		}
		return nil
	}

	requireServiceConfig = func(c *cli.Context) error {
		if c.String(confFlagName) == "" {
			return errors.New("service configuration path is not specified")
		}
		return nil
	}

	setPlainLogger = func(c *cli.Conetext) error {
		grip.CatchWarning(grip.SetSender(send.MakePlainLogger()))
	}

	requirePathFlag = func(c *cli.Context) error {
		path := c.String(pathFlagName)
		if path == "" {
			if c.NArg() != 1 {
				return errors.New("must specify the path to an evergreen configuration")
			}
			path = c.Arg().Get(0)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			return errors.Errorf("configuration file %s does not exist", path)
		}

		c.Set(pathFlagName, path)
	}
)

func mergeBeforeFuncs(ops ...func(c *cli.Context) error) cli.BeforeFunc {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()

		for _, op := range ops {
			catcher.Add(op())
		}

		return catcher.Resolve()
	}
}
