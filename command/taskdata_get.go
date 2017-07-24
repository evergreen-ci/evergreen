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

type taskDataGet struct {
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
	TaskName string `mapstructure:"task" plugin:"expand"`
	Variant  string `mapstructure:"variant" plugin:"expand"`
}

func taskDataGetFactory() Command   { return &taskDataGet{} }
func (c *taskDataGet) Name() string { return "json.get" }

func (c *taskDataGet) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", c.Name())
	}
	if c.File == "" {
		return errors.New("JSON 'get' command must not have blank 'file' parameter")
	}
	if c.DataName == "" {
		return errors.New("JSON 'get' command must not have a blank 'name' param")
	}
	if c.TaskName == "" {
		return errors.New("JSON 'get' command must not have a blank 'task' param")
	}

	return nil
}

func (c *taskDataGet) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	err := errors.WithStack(util.ExpandValues(c, conf.Expansions))
	if err != nil {
		return err
	}

	if c.File != "" && !filepath.IsAbs(c.File) {
		c.File = filepath.Join(conf.WorkDir, c.File)
	}
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	data, err := comm.GetJSONData(ctx, td, c.TaskName, c.DataName, c.Variant)
	if err != nil {
		return errors.Wrapf(err, "problem retrieving data from %s/%s/%s",
			c.TaskName, c.DataName, c.Variant)
	}

	return errors.Wrapf(ioutil.WriteFile(c.File, data, 0755),
		"problem writing data from task %s/%s/%s to file %s",
		c.TaskName, c.DataName, c.Variant, c.File)

}
