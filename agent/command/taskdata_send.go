package command

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type taskDataSend struct {
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
	base
}

func taskDataSendFactory() Command   { return &taskDataSend{} }
func (c *taskDataSend) Name() string { return "json.send" }

func (c *taskDataSend) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.File == "" {
		return errors.New("file name must not be blank")
	}

	if c.DataName == "" {
		return errors.New("name must not be blank")
	}

	return nil
}

func (c *taskDataSend) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	errChan := make(chan error)
	go func() {
		// attempt to open the file
		fileLoc := getJoinedWithWorkDir(conf, c.File)
		jsonFile, err := os.Open(fileLoc)
		if err != nil {
			errChan <- errors.Wrapf(err, "opening JSON file '%s'", fileLoc)
			return
		}

		jsonData := map[string]interface{}{}
		err = utility.ReadJSON(jsonFile, &jsonData)
		if err != nil {
			errChan <- errors.Wrapf(err, "reading JSON from file '%s'", fileLoc)
			return
		}

		errChan <- errors.Wrapf(comm.PostJSONData(ctx, td, c.DataName, jsonData),
			"posting task data for name '%s' (task '%s')", c.DataName, td.ID)
	}()

	select {
	case err := <-errChan:
		return errors.Wrap(err, "reading and sending JSON data")
	case <-ctx.Done():
		logger.Execution().Infof("Canceled command '%s' while reading and posting JSON data: %s", c.Name(), ctx.Err())
		return nil
	}
}
