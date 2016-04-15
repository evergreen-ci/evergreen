package attach

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"net/http"
	"time"
)

func init() {
	plugin.Publish(&AttachPlugin{})
}

const (
	AttachPluginName      = "attach"
	AttachResultsCmd      = "results"
	AttachXunitResultsCmd = "xunit_results"

	AttachResultsAPIEndpoint = "results"
	AttachLogsAPIEndpoint    = "test_logs"

	AttachResultsPostRetries   = 5
	AttachResultsRetrySleepSec = 10 * time.Second
)

// AttachPlugin has commands for uploading task results and links to files,
// for display and easy access in the UI.
type AttachPlugin struct{}

// Name returns the name of this plugin - it serves to satisfy
// the 'Plugin' interface
func (self *AttachPlugin) Name() string {
	return AttachPluginName
}

func (self *AttachPlugin) GetAPIHandler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc(fmt.Sprintf("/%v", AttachResultsAPIEndpoint), AttachResultsHandler)
	r.HandleFunc("/", http.NotFound) // 404 any request not routable to these endpoints
	return r
}

func (self *AttachPlugin) GetUIHandler() http.Handler {
	return nil
}

func (self *AttachPlugin) Configure(map[string]interface{}) error {
	return nil
}

// stripHiddenFiles is a helper for only showing users the files they are allowed to see.
func stripHiddenFiles(files []artifact.File, pluginUser *user.DBUser) []artifact.File {
	publicFiles := []artifact.File{}
	for _, file := range files {
		switch {
		case file.Visibility == artifact.None:
			continue
		case file.Visibility == artifact.Private && pluginUser == nil:
			continue
		default:
			publicFiles = append(publicFiles, file)
		}
	}
	return publicFiles
}

// GetPanelConfig returns a plugin.PanelConfig struct representing panels
// that will be added to the Task and Build pages.
func (self *AttachPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{
		Panels: []plugin.UIPanel{
			{
				Page:     plugin.TaskPage,
				Position: plugin.PageLeft,
				PanelHTML: "<div ng-include=\"'/plugin/attach/static/partials/task_files_panel.html'\" " +
					"ng-init='files=plugins.attach' ng-show='plugins.attach.length'></div>",
				DataFunc: func(context plugin.UIContext) (interface{}, error) {
					if context.Task == nil {
						return nil, nil
					}
					artifactEntry, err := artifact.FindOne(artifact.ByTaskId(context.Task.Id))
					if err != nil {
						return nil, fmt.Errorf("error finding artifact files for task: %v", err)
					}
					if artifactEntry == nil {
						return nil, nil
					}
					return stripHiddenFiles(artifactEntry.Files, context.User), nil
				},
			},
			{
				Page:     plugin.BuildPage,
				Position: plugin.PageLeft,
				PanelHTML: "<div ng-include=\"'/plugin/attach/static/partials/build_files_panel.html'\" " +
					"ng-init='filesByTask=plugins.attach' ng-show='plugins.attach.length'></div>",
				DataFunc: func(context plugin.UIContext) (interface{}, error) {
					if context.Build == nil {
						return nil, nil
					}
					taskArtifactFiles, err := artifact.FindAll(artifact.ByBuildId(context.Build.Id))
					if err != nil {
						return nil, fmt.Errorf("error finding artifact files for build: %v", err)
					}
					for i := range taskArtifactFiles {
						// remove hidden files if the user isn't logged in
						taskArtifactFiles[i].Files = stripHiddenFiles(taskArtifactFiles[i].Files, context.User)
					}
					return taskArtifactFiles, nil
				},
			},
		},
	}, nil
}

// NewCommand returns the AttachPlugin - this is to satisfy the
// 'Plugin' interface
func (self *AttachPlugin) NewCommand(cmdName string) (plugin.Command,
	error) {
	switch cmdName {
	case AttachResultsCmd:
		return &AttachResultsCommand{}, nil
	case AttachXunitResultsCmd:
		return &AttachXUnitResultsCommand{}, nil
	default:
		return nil, fmt.Errorf("No such %v command: %v",
			AttachPluginName, cmdName)
	}
}

// SendJSONLogs is responsible for sending the specified logs
// to the API Server. If successful, it returns a log ID that can be used
// to refer to the log object in test results.
func SendJSONLogs(taskConfig *model.TaskConfig, pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, logs *model.TestLog) (string, error) {
	pluginLogger.LogExecution(slogger.INFO, "Attaching test logs for %v", logs.Name)
	logId, err := pluginCom.TaskPostTestLog(logs)
	if err != nil {
		return "", err
	}
	pluginLogger.LogTask(slogger.INFO, "Attach test logs succeeded")
	return logId, nil
}

//XXX remove this once the transition is complete...
//AttachResultsHandler is an API hook for receiving and updating test results
func AttachResultsHandler(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		message := "Cannot find task for attach results request"
		evergreen.Logger.Errorf(slogger.ERROR, message)
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	results := &task.TestResults{}
	err := util.ReadJSONInto(r.Body, results)
	if err != nil {
		message := fmt.Sprintf("error reading test results: %v", err)
		evergreen.Logger.Errorf(slogger.ERROR, message)
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	// set test result of task
	if err := t.SetResults(results.Results); err != nil {
		message := fmt.Sprintf("Error calling set results on task %v: %v", t.Id, err)
		evergreen.Logger.Errorf(slogger.ERROR, message)
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, "Test results successfully attached")
}
