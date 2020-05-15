package operations

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Pull() cli.Command {
	const (
		taskFlagName = "task"
	)
	return cli.Command{
		Name:  "pull",
		Usage: "pull a completed task's exact working directory contents",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  joinFlagNames(taskFlagName, "t"),
				Usage: "the ID of the task to pull",
			},
			cli.StringFlag{
				Name:  joinFlagNames(dirFlagName, "d"),
				Usage: "the directory to put the task contents (default: current working directory)",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(taskFlagName),
			requireWorkingDirFlag(dirFlagName),
		),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			taskID := c.String(taskFlagName)
			workingDir := c.String(dirFlagName)

			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client := conf.setupRestCommunicator(ctx)
			defer client.Close()

			creds, err := client.GetTaskSyncReadCredentials(ctx)
			if err != nil {
				return errors.Wrap(err, "could not fetch credentials")
			}

			remotePath, err := client.GetTaskSyncPath(ctx, taskID)
			if err != nil {
				return errors.Wrap(err, "could not get location of task directory in S3")
			}

			httpClient := utility.GetDefaultHTTPRetryableClient()
			// Do not time out a download since it could be an expensive
			// operation depending on the download speed and size of the pull.
			httpClient.Timeout = 0
			defer utility.PutHTTPClient(httpClient)
			opts := pail.S3Options{
				Name:        creds.Bucket,
				Credentials: pail.CreateAWSCredentials(creds.Key, creds.Secret, ""),
				Region:      endpoints.UsEast1RegionID,
				Permissions: pail.S3PermissionsBucketOwnerRead,
				Verbose:     true,
			}
			bucket, err := pail.NewS3ArchiveBucketWithHTTPClient(httpClient, opts)
			if err != nil {
				return errors.Wrap(err, "error setting up S3 bucket")
			}

			if err = os.MkdirAll(workingDir, 0755); err != nil {
				return errors.Wrap(err, "could not make working directory")
			}

			_ = grip.SetSender(send.MakePlainLogger())

			grip.Infof("Beginning download for task '%s'\n", taskID)

			if err := bucket.Pull(ctx, pail.SyncOptions{
				Local:  workingDir,
				Remote: remotePath,
			}); err != nil {
				return errors.Wrap(err, "error while pulling task directory")
			}

			grip.Infof("Download complete.")

			return nil
		},
	}
}
