package command

import (
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type taskDataSend struct {
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
}

func taskDataSendFactory() Command     { return &taskDataSend{} }
func (c *taskDataSend) Name() string   { return "send" }
func (c *taskDataSend) Plugin() string { return "json" }

func (c *taskDataSend) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", c.Name())
	}

	if c.File == "" {
		return errors.New("'file' param must not be blank")
	}

	if c.DataName == "" {
		return errors.New("'name' param must not be blank")
	}

	return nil
}

func (c *taskDataSend) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	errChan := make(chan error)
	go func() {
		// attempt to open the file
		fileLoc := filepath.Join(conf.WorkDir, c.File)
		jsonFile, err := os.Open(fileLoc)
		if err != nil {
			errChan <- errors.Wrap(err, "Couldn't open json file")
			return
		}

		jsonData := map[string]interface{}{}
		err = util.ReadJSONInto(jsonFile, &jsonData)
		if err != nil {
			errChan <- errors.Wrap(err, "File contained invalid json")
			return
		}

		errChan <- errors.Wrapf(comm.PostJSONData(ctx, td, c.DataName, jsonData),
			"problem posting task data for %s (%s)", c.DataName, td.ID)
		return
	}()

	select {
	case err := <-errChan:
		if err != nil {
			logger.Task().Errorf("Sending json data failed: %v", err)
		}
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Info("Received abort signal, stopping.")
		return nil
	}
}
