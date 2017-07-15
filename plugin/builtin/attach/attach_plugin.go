package attach

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/pkg/errors"
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
						return nil, errors.Wrap(err, "error finding artifact files for task")
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
						return nil, errors.Wrap(err, "error finding artifact files for build")
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
