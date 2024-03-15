package command

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type listHosts struct {
	Path        string `mapstructure:"path" plugin:"expand"`
	Wait        bool   `mapstructure:"wait"`
	Silent      bool   `mapstructure:"silent"`
	TimeoutSecs int    `mapstructure:"timeout_seconds"`
	NumHosts    string `mapstructure:"num_hosts" plugin:"expand"`
	base
}

func listHostFactory() Command  { return &listHosts{} }
func (*listHosts) Name() string { return "host.list" }
func (c *listHosts) ParseParams(params map[string]interface{}) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           c,
	})
	if err != nil {
		return errors.Wrap(err, "constructing mapstructure decoder")
	}
	if err := decoder.Decode(params); err != nil {
		return errors.Wrap(err, "decoding mapstrcuture parameters")
	}

	if c.Wait && (c.NumHosts == "" || c.NumHosts == "0") {
		return errors.New("cannot wait for 0 hosts")
	}

	if c.Path == "" && c.Silent && !c.Wait {
		return errors.New("if not waiting for hosts to come up, must specify an output destination (either a file path or non-silent log output)")
	}

	if c.TimeoutSecs < 0 {
		return errors.New("cannot have negative timeout")
	}

	return nil
}

func (c *listHosts) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// check for this during execution too encase the expansion of
	// path was empty
	if c.Path == "" && c.Silent && !c.Wait {
		return errors.New("if not waiting for hosts to come up, must specify an output destination (either a file path or non-silent log output)")
	}

	if c.Path != "" {
		if !filepath.IsAbs(c.Path) {
			c.Path = GetWorkingDirectory(conf, c.Path)
		}
	}

	if c.TimeoutSecs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.TimeoutSecs)*time.Second)
		defer cancel()
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	var results restmodel.HostListResults
	var err error
	var timeout bool

	backoffCounter := getS3OpBackoff()
	timer := time.NewTimer(0)
	defer timer.Stop()

	var numHosts int
	if c.NumHosts != "" {
		numHosts, err = strconv.Atoi(c.NumHosts)
		if err != nil {
			return errors.Wrapf(err, "converting num hosts '%s' to int", c.NumHosts)
		}
	}

waitForHosts:
	for {
		select {
		case <-ctx.Done():
			timeout = true
			break waitForHosts
		case <-timer.C:
			results, err = comm.ListHosts(ctx, td)

			if c.Wait { // all hosts are up or failed to come up
				if err == nil && numHosts <= (len(results.Hosts)+len(results.Details)) {
					break waitForHosts
				} else if err != nil {
					// pass
				} else {
					err = errors.Errorf("%d hosts of %d are up, waiting", len(results.Hosts), numHosts)
				}
			} else if err == nil {
				break waitForHosts
			}
			timer.Reset(backoffCounter.Duration())
		}
	}

	if err != nil {
		return errors.Wrap(err, "getting host statuses")
	}

	if c.Path != "" {
		if len(results.Hosts) > 0 {
			if err = utility.WriteJSONFile(c.Path, results.Hosts); err != nil {
				return errors.Wrapf(err, "writing host data to file '%s'", c.Path)
			}
		}
		if len(results.Details) > 0 {
			if err = utility.WriteJSONFile(c.Path, results.Details); err != nil {
				return errors.Wrapf(err, "writing host failure details to file '%s'", c.Path)
			}
		}
	}

	if !c.Silent {
		if len(results.Hosts) > 0 {
			jsonBytes, err := json.MarshalIndent(results.Hosts, "  ", "   ")
			logger.Task().Warning(errors.Wrap(err, "formatting host data as JSON"))
			logger.Task().Info(string(jsonBytes))
		}
		if len(results.Details) > 0 {
			jsonBytes, err := json.MarshalIndent(results.Details, "  ", "   ")
			logger.Task().Warning(errors.Wrap(err, "formatting host failure details as JSON"))
			logger.Task().Info(string(jsonBytes))
		}
	}
	if timeout {
		return errors.Errorf("reached timeout (%d seconds) waiting for hosts", c.TimeoutSecs)
	}
	if len(results.Details) > 0 {
		return errors.Errorf("%d hosts of %d failed", len(results.Details), numHosts)
	}

	return nil
}
