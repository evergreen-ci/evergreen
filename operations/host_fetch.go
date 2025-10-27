package operations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func hostFetch() cli.Command {
	return cli.Command{
		Name:  "fetch",
		Usage: "runs the fetch script on a spawn host",
		Flags: []cli.Flag{},
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			confPath := c.Parent().String(confFlagName)

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			// Setting up the REST communicator enforces that
			// the user is authenticated before running the fetch script.
			client, err := conf.setupRestCommunicator(ctx, true)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer client.Close()

			userHome, err := util.GetUserHome()
			if err != nil {
				return errors.Wrap(err, "getting user home directory")
			}
			spawnHostFetchScript := filepath.Join(userHome, evergreen.SpawnhostFetchScriptName)

			fetchScriptBytes, err := os.ReadFile(spawnHostFetchScript)
			if err != nil {
				return errors.Wrap(err, "reading fetch script")
			}
			fetchScript := string(fetchScriptBytes)

			inputMarker := "<<EOF"
			inputStart := strings.Index(fetchScript, inputMarker)

			output := util.NewMBCappedWriter()
			command := fetchScript[0:inputStart]
			cmd := jasper.NewCommand().Add(strings.Split(command, " ")).SetCombinedWriter(output)

			input := fetchScript[inputStart+len(inputMarker):]
			cmd.SetInput(strings.NewReader(input))

			if err = cmd.Run(ctx); err != nil {
				return errors.Wrapf(err, "running command: %s", output.String())
			}

			fmt.Println("========== Output from fetch script ==========")
			fmt.Println(output)
			fmt.Println("=============================================")

			return nil
		},
	}
}
