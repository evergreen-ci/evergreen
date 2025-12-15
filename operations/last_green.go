package operations

import (
	"os"
	"text/template"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
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
		Before: mergeBeforeFuncs(autoUpdateCLI, requireVariantsFlag),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			variants := c.StringSlice(variantsFlagName)
			project := c.String(projectFlagName)

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			_, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			v, err := rc.GetLastGreen(project, variants)
			if err != nil {
				return err
			}

			return lastGreenTemplate.Execute(os.Stdout, struct {
				Version *model.Version
				UIURL   string
			}{v, conf.UIServerHost})
		},
	}
}
