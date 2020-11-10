package command

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/pkg/errors"
)

func moduleRevExpansionName(name string) string { return fmt.Sprintf("%s_rev", name) }

// manifestLoad
type manifestLoad struct{ base }

func manifestLoadFactory() Command   { return &manifestLoad{} }
func (c *manifestLoad) Name() string { return "manifest.load" }

// manifestLoad-specific implementation of ParseParams.
func (c *manifestLoad) ParseParams(params map[string]interface{}) error {
	return nil
}

// Load performs a GET on /manifest/load
func (c *manifestLoad) Load(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	manifest, err := comm.GetManifest(ctx, td)
	if err != nil {
		return errors.Wrapf(err, "problem loading manifest for %s", td.ID)
	}

	for moduleName := range manifest.Modules {
		// put the url for the module in the expansions
		conf.Expansions.Put(moduleRevExpansionName(moduleName), manifest.Modules[moduleName].Revision)
		conf.Expansions.Put(fmt.Sprintf("%s_branch", moduleName), manifest.Modules[moduleName].Branch)
		conf.Expansions.Put(fmt.Sprintf("%s_repo", moduleName), manifest.Modules[moduleName].Repo)
		conf.Expansions.Put(fmt.Sprintf("%s_owner", moduleName), manifest.Modules[moduleName].Owner)
	}

	logger.Execution().Info("manifest loaded successfully")
	return nil
}

// Implementation of Execute.
func (c *manifestLoad) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

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
