package plugin

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
							var execTaskFiles []artifact.File
							execTaskFiles, err = artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: execTaskID, Execution: context.Task.Execution}})
							if err != nil {
								return nil, err
							}
							hasUser := context.User.(*user.DBUser) != nil

							grip.Debug(message.Fields{
								"message":       "Chaya. attache_plugin  77",
								"execTaskFiles": execTaskFiles,
								"stack":         message.NewStack(1, "").Raw(),
								"hasuser":       hasUser,
								"user":          context.User,
								"user == nil":   context.User == nil,
							})
							strippedFiles, err := artifact.StripHiddenFiles(execTaskFiles, hasUser)
							grip.Debug(message.Fields{
								"message":       "Chaya. attach_plugin 87 after calling stripHidden",
								"execTaskFiles": execTaskFiles,
								"stack":         message.NewStack(1, "").Raw(),
								"hasUser":       hasUser,
								"user":          context.User,
								"user == nil":   context.User == nil,
							})
							if err != nil {
								return nil, errors.Wrap(err, "error signign urls")
							}
							var execTask *task.Task
							execTask, err = task.FindOne(task.ById(execTaskID))
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

					files, err := artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: taskId, Execution: context.Task.Execution}})
					if err != nil {
						return nil, err
					}
					hasUser := context.User.(*user.DBUser) != nil

					// var currentUser gimlet.User = context.User
					if context.User.DisplayName != nil {
						grip.Debug(message.Fields{
							"err":     err,
							"message": "Chaya. attach_plugin  123",
							"files":   files,
							"stack":   message.NewStack(1, "").Raw(),
							"hasUser": hasUser,
							"user":    context.User,
						})
					} else {

						grip.Debug(message.Fields{
							"err":         err,
							"message":     "Chaya. attach_plugin  139. username is nil",
							"files":       files,
							"stack":       message.NewStack(1, "").Raw(),
							"hasUser":     hasUser,
							"user":        context.User,
							"user == nil": context.User == nil,
						})
					}

					strippedFiles, err := artifact.StripHiddenFiles(files, hasUser)

					grip.Debug(message.Fields{
						"err":         err,
						"message":     "Chaya. attach_plugin  145, after Striphiddenfiles",
						"files":       files,
						"stack":       message.NewStack(1, "").Raw(),
						"hasUser":     hasUser,
						"user":        context.User,
						"user == nil": context.User.(*user.DBUser) == nil,
					})
					if err != nil {
						return nil, errors.Wrap(err, "error signing urls")
					}
					return strippedFiles, nil
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
						hasUser := context.User.(*user.DBUser) != nil
						grip.Debug(message.Fields{
							"err":         err,
							"message":     "Chaya. attach_plugin  158, before  Striphiddenfiles",
							"files":       taskArtifactFiles,
							"stack":       message.NewStack(1, "").Raw(),
							"hasUser":     hasUser,
							"user":        context.User,
							"user == nil": context.User == nil,
						})
						taskArtifactFiles[i].Files, err = artifact.StripHiddenFiles(taskArtifactFiles[i].Files, hasUser)
						grip.Debug(message.Fields{
							"err":         err,
							"message":     "Chaya. attach_plugin  167, after Striphiddenfiles",
							"files":       taskArtifactFiles,
							"stack":       message.NewStack(1, "").Raw(),
							"hasUser":     hasUser,
							"user":        context.User,
							"user == nil": context.User == nil,
						})
						if err != nil {
							return nil, errors.Wrap(err, "error singing urls")
						}
					}
					return taskArtifactFiles, nil
				},
			},
		},
	}, nil
}
