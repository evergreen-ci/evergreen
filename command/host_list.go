package command

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type listHosts struct {
	Path        string `mapstructure:"path" plugin:"expand"`
	Wait        bool   `mapstructure:"wait"`
	Silent      bool   `mapstructure:"silent"`
	TimeoutSecs int    `mapstructure:"timeout_seconds"`
	NumHosts    int    `mapstructure:"num_hosts"`
	base
}

func listHostFactory() Command  { return &listHosts{} }
func (*listHosts) Name() string { return "host.list" }
func (c *listHosts) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c.CreateHost); err != nil {
		return errors.Wrapf(err, "error parsing '%s' params", c.Name())
	}

	if c.Wait && c.NumHosts == 0 {
		return errors.New("cannot reasonably wait for 0 hosts")

	}

	if c.Path == "" && c.Silent && !c.Wait {
		return errors.New("unreasonable combination of output, silent, and wait options")
	}

	if c.TimeoutSecs < 0 {
		return errors.New("unreasonable timeout value")
	}

	return nil
}

func (c *listHosts) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	// expand the S3 copy parameters before running the task
	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.WithStack(err)
	}

	// check for this during execution too encase the expansion of
	// path was empty
	if c.Path == "" && c.Silent && !c.Wait {
		return errors.New("unreasonable combination of output, silent, and wait options")
	}

	if c.Path != "" {
		if !filepath.IsAbs(c.LocalFile) {
			c.Path = filepath.Join(conf.WorkDir, c.Path)
		}
	}

	if c.TimeoutSecs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.TimeoutSecs*time.Second)
		defer cancel()
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	var hosts []restmodel.APIHost
	var err error

	backoffCounter := getS3OpBackoff()
	timer := time.NewTimer(0)
	defer timer.Stop()

waitForHosts:
	for {
		select {
		case <-ctx.Done():
			err = errors.New("reached timeout")
			break waitForHosts
		case <-timer.C:
			hosts, err = comm.ListHosts(ctx, td)

			if c.Wait {
				if err == nil && c.NumHosts == len(hosts) {
					break waitForHosts
				} else if err != nil {
					// pass
				} else {
					err = errors.Errorf("%d hosts of %d are up, waiting", len(hosts), c.NumHosts)
				}
			} else if err == nil {
				break waitForHosts
			}
			timer.Reset(backoffCounter.Reset())
		}
	}

	if err != nil {
		return errors.Wrap(err, "reached timeout waiting for hosts ")
	}

	if c.Path != "" {
		if err = util.WriteJSONInto(c.Path, hosts); err != nil {
			return errors.Wrapf(err, "problem writing host data to file: %s", c.Path)
		}
	}

	if !c.Silent {
		jsonBytes, err := json.MarshalIndent(hosts, "  ", "   ")
		logger.Task().Warning(errors.Wrap(err, "problem json formatting url"))
		logger.Task().Info(string(jsonBytes))
	}

	return nil
}
