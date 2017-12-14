package operations

import (
	"errors"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/urfave/cli"
)

var (
	requireConfig = func(c *cli.Context) error {
		if c.String("config") == "" {
			catcher.Add(errors.New("command line configuration path is not specified"))
		}
	}

	setPlainLogger = func(c *cli.Conetext) error {
		grip.CatchWarning(grip.SetSender(send.MakePlainLogger()))
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
