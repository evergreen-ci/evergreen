package manifest

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

// ManifestLoadCommand
type ManifestLoadCommand struct{}

func (mfc *ManifestLoadCommand) Name() string {
	return ManifestLoadCmd
}

func (mfc *ManifestLoadCommand) Plugin() string {
	return ManifestPluginName
}

// ManifestLoadCommand-specific implementation of ParseParams.
func (mfc *ManifestLoadCommand) ParseParams(params map[string]interface{}) error {
	return nil
}

// updateExpansions adds the expansions for the manifest's modules into the TaskConfig's Expansions.
func (mfc *ManifestLoadCommand) updateExpansions(manifest *manifest.Manifest,
	conf *model.TaskConfig) {
	for moduleName := range manifest.Modules {
		// put the url for the module in the expansions
		conf.Expansions.Put(fmt.Sprintf("%v_rev", moduleName), manifest.Modules[moduleName].Revision)
	}
}

// Load performs a GET on /manifest/load
func (mfc *ManifestLoadCommand) Load(log plugin.Logger, pluginCom plugin.PluginCommunicator, conf *model.TaskConfig) error {
	var loadedManifest *manifest.Manifest
	var err error

	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := pluginCom.TaskGetJSON(ManifestLoadAPIEndpoint)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				log.LogExecution(slogger.WARN, "Error connecting to API server: %v", err)
				return util.RetriableError{err}
			}
			if resp != nil && resp.StatusCode != http.StatusOK {
				log.LogExecution(slogger.WARN, "Unexpected status code %v, retrying", resp.StatusCode)
				return util.RetriableError{errors.Errorf("Unexpected status code %v", resp.StatusCode)}
			}
			err = util.ReadJSONInto(resp.Body, &loadedManifest)
			return err
		})

	_, err = util.Retry(retriableGet, 10, 1*time.Second)
	if err != nil {
		return err
	}
	if loadedManifest == nil {
		return errors.New("Manifest is empty")
	}
	mfc.updateExpansions(loadedManifest, conf)
	return nil
}

// Implementation of Execute.
func (mfc *ManifestLoadCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig,
	stop chan bool) error {

	errChan := make(chan error)
	go func() {
		errChan <- mfc.Load(pluginLogger, pluginCom, conf)
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of manifest load command")
		return nil
	}

}
