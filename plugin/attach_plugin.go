package plugin

import (
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/pkg/errors"
)

func init() {
	Publish(&AttachPlugin{})
}

const AttachPluginName = "attach"

// AttachPlugin has commands for uploading task results and links to files,
// for display and easy access in the UI.
type AttachPlugin struct{}

type displayTaskFiles struct {
	Name  string
	Files []artifact.File
}

// Name returns the name of this plugin - it serves to satisfy
// the 'Plugin' interface
func (ap *AttachPlugin) Name() string                           { return AttachPluginName }
func (ap *AttachPlugin) Configure(map[string]interface{}) error { return nil }

// GetPanelConfig returns a plugin.PanelConfig struct representing panels
// that will be added to the Task and Build pages.
func (ap *AttachPlugin) GetPanelConfig() (*PanelConfig, error) {
	return &PanelConfig{
		Panels: []UIPanel{
			{
				Page:     TaskPage,
				Position: PageLeft,
				PanelHTML: "<div ng-include=\"'/static/plugins/attach/partials/task_files_panel.html'\" " +
					"ng-init='entries=plugins.attach' ng-show='plugins.attach.length'></div>",
				DataFunc: func(uiCtx UIContext) (interface{}, error) {
					if uiCtx.Task == nil {
						return nil, nil
					}
					var err error
					taskId := uiCtx.Task.Id
					t := uiCtx.Task
					if uiCtx.Task.OldTaskId != "" {
						taskId = uiCtx.Task.OldTaskId
						t, err = task.FindOneId(uiCtx.Request.Context(), taskId)
						if err != nil {
							return nil, errors.Wrap(err, "error retrieving task")
						}
					}

					if t.DisplayOnly {
						files := []displayTaskFiles{}
						for _, execTaskID := range t.ExecutionTasks {
							var execTaskFiles []artifact.File
							execTaskFiles, err = artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: execTaskID, Execution: uiCtx.Task.Execution}})
							if err != nil {
								return nil, err
							}
							hasUser := uiCtx.User.(*user.DBUser) != nil
							var strippedFiles []artifact.File

							strippedFiles, err = artifact.StripHiddenFiles(uiCtx.Request.Context(), execTaskFiles, hasUser)
							if err != nil {
								return nil, errors.Wrap(err, "signing urls")
							}
							var execTask *task.Task
							execTask, err = task.FindOneId(uiCtx.Request.Context(), execTaskID)
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

					files, err := artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: taskId, Execution: uiCtx.Task.Execution}})
					if err != nil {
						return nil, err
					}
					hasUser := uiCtx.User.(*user.DBUser) != nil

					strippedFiles, err := artifact.StripHiddenFiles(uiCtx.Request.Context(), files, hasUser)
					if err != nil {
						return nil, errors.Wrap(err, "signing urls")
					}
					return strippedFiles, nil
				},
			},
			{
				Page:     BuildPage,
				Position: PageLeft,
				PanelHTML: "<div ng-include=\"'/static/plugins/attach/partials/build_files_panel.html'\" " +
					"ng-init='filesByTask=plugins.attach' ng-show='plugins.attach.length'></div>",
				DataFunc: func(uiCtx UIContext) (interface{}, error) {
					if uiCtx.Build == nil {
						return nil, nil
					}
					taskArtifactFiles, err := artifact.FindAll(artifact.ByBuildId(uiCtx.Build.Id))
					if err != nil {
						return nil, errors.Wrap(err, "error finding artifact files for build")
					}
					for i := range taskArtifactFiles {
						// remove hidden files if the user isn't logged in
						hasUser := uiCtx.User.(*user.DBUser) != nil
						taskArtifactFiles[i].Files, err = artifact.StripHiddenFiles(uiCtx.Request.Context(), taskArtifactFiles[i].Files, hasUser)
						if err != nil {
							return nil, errors.Wrap(err, "signing urls")
						}
					}
					return taskArtifactFiles, nil
				},
			},
		},
	}, nil
}
