package attach

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/mapstructure"
	"io/ioutil"
	"net/http"
)

// AttachTaskFilesCommand sends a mapping of filename -> link to the
// api server, so users can attach arbitrary files and links
// to a task
type AttachTaskFilesCommand struct {
	// Files is a map of strings to strings storing
	// name -> link pairs. These pairs are sent to the
	// server and attached to a task.
	Files model.ArtifactFileParams
}

func (self *AttachTaskFilesCommand) Name() string {
	return AttachTaskFilesCmd
}

// ParseParams decodes the S3 push command parameters that are
// specified as part of an AttachPlugin command; this is required
// to satisfy the 'Command' interface
func (self *AttachTaskFilesCommand) ParseParams(
	params map[string]interface{}) error {
	if err := mapstructure.Decode(params, &self.Files); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", self.Name(), err)
	}
	return nil
}

func (self *AttachTaskFilesCommand) expandAttachTaskFilesCommand(
	taskConfig *model.TaskConfig) (err error) {
	return plugin.ExpandValues(&self.Files, taskConfig.Expansions)
}

// Execute carries out the AttachResultsCommand command - this is required
// to satisfy the 'Command' interface
func (self *AttachTaskFilesCommand) Execute(pluginLogger plugin.PluginLogger,
	pluginCom plugin.PluginCommunicator,
	taskConfig *model.TaskConfig,
	stop chan bool) error {

	if err := self.expandAttachTaskFilesCommand(taskConfig); err != nil {
		msg := fmt.Sprintf("error expanding params: %v", err)
		pluginLogger.LogTask(slogger.ERROR, "Error updating task files: %v", msg)
		return fmt.Errorf(msg)
	}

	pluginLogger.LogTask(slogger.INFO, "Sending task file links to server")
	return self.SendTaskFiles(taskConfig, pluginLogger, pluginCom)
}

// SendJSONResults is responsible for sending the
// specified file to the API Server
func (self *AttachTaskFilesCommand) SendTaskFiles(taskConfig *model.TaskConfig,
	pluginLogger plugin.PluginLogger, pluginCom plugin.PluginCommunicator) error {

	// log each file attachment
	for name, link := range self.Files {
		pluginLogger.LogTask(slogger.INFO,
			"Attaching file: %v -> %v", name, link)
	}

	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := pluginCom.TaskPostJSON(
				AttachTaskFilesAPIEndpoint,
				self.Files.Array(),
			)
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				body, _ := ioutil.ReadAll(resp.Body)
				msg := fmt.Sprintf(
					"secret conflict while posting task files: %v", string(body))
				pluginLogger.LogTask(slogger.ERROR, msg)
				return fmt.Errorf(msg)
			}
			if resp != nil && resp.StatusCode == http.StatusBadRequest {
				body, _ := ioutil.ReadAll(resp.Body)
				msg := fmt.Sprintf(
					"error posting task files (%v): %v", resp.StatusCode, string(body))
				pluginLogger.LogTask(slogger.ERROR, msg)
				return fmt.Errorf(msg)
			}
			if resp != nil && resp.StatusCode != http.StatusOK {
				body, _ := ioutil.ReadAll(resp.Body)
				msg := fmt.Sprintf("error posting task files (%v): %v",
					resp.StatusCode, string(body))
				pluginLogger.LogExecution(slogger.WARN, msg)
				return util.RetriableError{err}
			}
			if err != nil {
				msg := fmt.Sprintf("error posting files: %v", err)
				pluginLogger.LogExecution(slogger.WARN, msg)
				return util.RetriableError{fmt.Errorf(msg)}
			}
			return nil
		},
	)

	retryFail, err := util.Retry(retriableSendFile, AttachResultsPostRetries,
		AttachResultsRetrySleepSec)

	if retryFail {
		return fmt.Errorf("Attach files failed after %v tries: %v",
			AttachResultsPostRetries, err)
	}
	if err != nil {
		return fmt.Errorf("Attach files failed: %v", err)
	}

	pluginLogger.LogExecution(slogger.INFO, "API attach files call succeeded")
	return nil
}

// AttachFilesHandler updates file mappings for a task or build
func AttachFilesHandler(w http.ResponseWriter, r *http.Request) {
	task := plugin.GetTask(r)
	mci.Logger.Logf(slogger.INFO, "Attaching files to task %v", task.Id)

	fileEntry := &model.ArtifactFileEntry{}
	fileEntry.TaskId = task.Id
	fileEntry.TaskDisplayName = task.DisplayName
	fileEntry.BuildId = task.BuildId

	err := util.ReadJSONInto(r.Body, &fileEntry.Files)
	if err != nil {
		message := fmt.Sprintf("error reading file definitions: %v", err)
		mci.Logger.Errorf(slogger.ERROR, message)
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	if err := fileEntry.Upsert(); err != nil {
		message := fmt.Sprintf("Error updating artifact file info: %v", err)
		mci.Logger.Errorf(slogger.ERROR, message)
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	plugin.WriteJSON(w, http.StatusOK, fmt.Sprintf("Artifact files for task %v successfully attached", task.Id))
}
