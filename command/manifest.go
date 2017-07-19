package command

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// manifestLoad
type manifestLoad struct{}

func manifestLoadFactory() Command     { return &manifestLoad{} }
func (c *manifestLoad) Name() string   { return "load" }
func (c *manifestLoad) Plugin() string { return "manifest" }

// manifestLoad-specific implementation of ParseParams.
func (c *manifestLoad) ParseParams(params map[string]interface{}) error {
	return nil
}

// Load performs a GET on /manifest/load
func (c *manifestLoad) Load(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	manifest, err := comm.GetManifest(ctx, td)
	if err != nil {
		return errors.Wrapf(err, "problem loading manifest for %s", td.ID)
	}

	for moduleName := range manifest.Modules {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		// put the url for the module in the expansions
		conf.Expansions.Put(fmt.Sprintf("%v_rev", moduleName), manifest.Modules[moduleName].Revision)
	}

	logger.Execution().Info("manifest loaded successfully")
	return nil
}

// Implementation of Execute.
func (c *manifestLoad) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	errChan := make(chan error)
	go func() {
		errChan <- c.Load(ctx, comm, logger, conf)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of manifest load command")
		return nil
	}

}
