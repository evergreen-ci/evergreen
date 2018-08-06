package plugin

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func init() {
	Publish(&AttachPlugin{})
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

type displayTaskFiles struct {
	Name  string
	Files []artifact.File
}

// Name returns the name of this plugin - it serves to satisfy
// the 'Plugin' interface
func (self *AttachPlugin) Name() string                           { return AttachPluginName }
func (self *AttachPlugin) Configure(map[string]interface{}) error { return nil }

// stripHiddenFiles is a helper for only showing users the files they are allowed to see.
func stripHiddenFiles(files []artifact.File, pluginUser gimlet.User) []artifact.File {
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

func getAllArtifacts(tasks []artifact.TaskIDAndExecution) ([]artifact.File, error) {
	artifacts, err := artifact.FindAll(artifact.ByTaskIdsAndExecutions(tasks))
	if err != nil {
		return nil, errors.Wrap(err, "error finding artifact files for task")
	}
	if artifacts == nil {
		taskIds := []string{}
		for _, t := range tasks {
			taskIds = append(taskIds, t.TaskID)
		}
		artifacts, err = artifact.FindAll(artifact.ByTaskIds(taskIds))
		if err != nil {
			return nil, errors.Wrap(err, "error finding artifact files for task without execution number")
		}
		if artifacts == nil {
			return []artifact.File{}, nil
		}
	}
	files := []artifact.File{}
	for _, artifact := range artifacts {
		files = append(files, artifact.Files...)
	}
	return files, nil
}

// GetPanelConfig returns a plugin.PanelConfig struct representing panels
// that will be added to the Task and Build pages.
func (self *AttachPlugin) GetPanelConfig() (*PanelConfig, error) {
	return &PanelConfig{
		Panels: []UIPanel{
			{
				Page:     TaskPage,
				Position: PageLeft,
				PanelHTML: "<div ng-include=\"'/static/plugins/attach/partials/task_files_panel.html'\" " +
					"ng-init='entries=plugins.attach' ng-show='plugins.attach.length'></div>",
				DataFunc: func(context UIContext) (interface{}, error) {
					if context.Task == nil {
						return nil, nil
					}
					var err error
					taskId := context.Task.Id
					t := context.Task
					if context.Task.OldTaskId != "" {
						taskId = context.Task.OldTaskId
						t, err = task.FindOneId(taskId)
						if err != nil {
							return nil, errors.Wrap(err, "error retrieving task")
						}
					}

					if t.DisplayOnly {
						files := []displayTaskFiles{}
						for _, execTaskID := range t.ExecutionTasks {
							execTaskFiles, err := getAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: execTaskID, Execution: context.Task.Execution}})
							if err != nil {
								return nil, err
							}
							strippedFiles := stripHiddenFiles(execTaskFiles, context.User)
							execTask, err := task.FindOne(task.ById(execTaskID))
							if err != nil {
								return nil, err
							}
							if execTask == nil {
								continue
							}
							if len(strippedFiles) > 0 {
								files = append(files, displayTaskFiles{
									Name:  execTask.DisplayName,
									Files: strippedFiles,
								})
							}
						}

						return files, nil
					}

					files, err := getAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: taskId, Execution: context.Task.Execution}})
					if err != nil {
						return nil, err
					}

					return stripHiddenFiles(files, context.User), nil
				},
			},
			{
				Page:     BuildPage,
				Position: PageLeft,
				PanelHTML: "<div ng-include=\"'/static/plugins/attach/partials/build_files_panel.html'\" " +
					"ng-init='filesByTask=plugins.attach' ng-show='plugins.attach.length'></div>",
				DataFunc: func(context UIContext) (interface{}, error) {
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
