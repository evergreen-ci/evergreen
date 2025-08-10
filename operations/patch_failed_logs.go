package operations

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// PatchFailedLogs is a convenience CLI to fetch logs for failed tasks in a patch.
func PatchFailedLogs() cli.Command {
	const (
		tailFlagName = "tail"
	)

	return cli.Command{
		Name:  "patch-failed-logs",
		Usage: "fetch logs for all failed tasks in a patch",
		Flags: mergeFlagSlices(
			addPatchIDFlag(
				cli.IntFlag{
					Name:  tailFlagName,
					Usage: "tail limit (number of lines) for logs per task",
					Value: 200,
				},
			),
		),
		Before: requirePatchIDFlag,
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			patchID := c.String(patchIDFlagName)
			tail := c.Int(tailFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			rest, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer rest.Close()

			ac, _, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "setting up legacy Evergreen client")
			}

			server := &mcpServer{conf: conf, rest: rest, ac: ac}

			failedTasks, err := server.listTasksForPatch(ctx, patchID, true)
			if err != nil {
				return errors.Wrap(err, "listing failed tasks")
			}

			for _, t := range failedTasks {
				content, err := server.getTaskLogs(ctx, t.ID, utility.ToIntPtr(t.Execution), "all_logs", tail)
				if err != nil {
					fmt.Fprintf(os.Stderr, "# %s (exec %d) ERROR: %v\n", t.ID, t.Execution, err)
					continue
				}
				fmt.Printf("# %s (%s on %s)\n", t.Display, t.Status, t.Variant)
				io.Copy(os.Stdout, strings.NewReader(content))
				fmt.Println()
			}

			return nil
		},
	}
}
