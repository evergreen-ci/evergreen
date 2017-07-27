package command

import (
	"io/ioutil"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type taskDataHistory struct {
	Tags     bool   `mapstructure:"tags"`
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
	TaskName string `mapstructure:"task" plugin:"expand"`
}

func taskDataHistoryFactory() Command   { return &taskDataHistory{} }
func (c *taskDataHistory) Name() string { return "json.get_history" }

func (c *taskDataHistory) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", c.Name())
	}
	if c.File == "" {
		return errors.New("JSON 'history' command must not have blank 'file' param")
	}
	if c.DataName == "" {
		return errors.New("JSON 'history command must not have blank 'name' param")
	}
	if c.TaskName == "" {
		return errors.New("JSON 'history command must not have blank 'task' param")
	}

	return nil
}

func (c *taskDataHistory) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	err := errors.WithStack(util.ExpandValues(c, conf.Expansions))
	if err != nil {
		return err
	}

	if c.File != "" && !filepath.IsAbs(c.File) {
		c.File = filepath.Join(conf.WorkDir, c.File)
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	data, err := comm.GetJSONHistory(ctx, td, c.Tags, c.TaskName, c.DataName)
	if err != nil {
		return errors.Wrapf(err, "problem getting data [%s/%s] from API server for %s",
			c.TaskName, c.DataName, td.ID)
	}

	return errors.Wrapf(ioutil.WriteFile(c.File, data, 0755),
		"problem writing json data for %s from %s/%s to %s",
		td.ID, c.TaskName, c.DataName, c.File)
}
