package operations

import (
	"context"
	"errors"
	"os"
	"text/template"

	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/urfave/cli"
)

var lastGreenTemplate = template.Must(template.New("last_green").Parse(`
   Revision : {{.Version.Revision}}
    Message : {{.Version.Message}}
       Link : {{.UIURL}}/version/{{.Version.Id}}

`))

func LastGreen() cli.Command {
	return cli.Command{
		Name:   "last-green",
		Usage:  "return a project's most recent successful version for given variants",
		Flags:  addProjectFlag(addVariantsFlag()...),
		Before: requireVariantsFlag,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			variants := c.StringSlice(variantsFlagName)
			project := c.String(projectFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			_ = conf.GetRestCommunicator(ctx)

			ac, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			notifyUserUpdate(ac)

			v, err := rc.GetLastGreen(project, variants)
			if err != nil {
				return err
			}

			return lastGreenTemplate.Execute(os.Stdout, struct {
				Version *version.Version
				UIURL   string
			}{v, conf.UIServerHost})
		},
	}
}
