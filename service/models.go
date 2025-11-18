package service

import (
	"html/template"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mongodb/grip"
)

type pluginData struct {
	Includes []template.HTML
	Panels   plugin.PanelLayout
	Data     map[string]any
}

type uiUpstreamData struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Revision    string `json:"revision"`
	ProjectName string `json:"project_name"`
	TriggerID   string `json:"trigger_id"`
	TriggerType string `json:"trigger_type"`
}

type uiPatch struct {
	Patch       patch.Patch
	StatusDiffs any

	// only used on task pages
	BaseTimeTaken time.Duration `json:"base_time_taken"`

	// for linking to other pages
	BaseVersionId string
	BaseBuildId   string
	BaseTaskId    string
}

type uiBuild struct {
	Build           build.Build
	Version         model.Version
	PatchInfo       *uiPatch `json:",omitempty"`
	Tasks           []uiTask
	Elapsed         time.Duration
	TimeTaken       time.Duration `json:"time_taken"`
	Makespan        time.Duration `json:"makespan"`
	CurrentTime     int64
	RepoOwner       string               `json:"repo_owner"`
	Repo            string               `json:"repo_name"`
	TaskStatusCount task.TaskStatusCount `json:"taskStatusCount"`
	UpstreamData    *uiUpstreamData      `json:"upstream_data,omitempty"`
}

type uiTask struct {
	Task             task.Task
	Gitspec          string
	BuildDisplay     string
	TaskLog          []apimodels.LogMessage
	NextTasks        []task.Task
	PreviousTasks    []task.Task
	Elapsed          time.Duration
	StartTime        int64
	FailedTestNames  []string      `json:"failed_test_names"`
	ExpectedDuration time.Duration `json:"expected_duration"`
}

///////////////////////////////////////////////////////////////////////////
//// Functions to create and populate the models
///////////////////////////////////////////////////////////////////////////

// getPluginDataAndHTML returns all data needed to properly render plugins
// for a page. It logs errors but does not return them, as plugin errors
// cannot stop the rendering of the rest of the page
func getPluginDataAndHTML(pluginManager plugin.PanelManager, page plugin.PageScope, ctx plugin.UIContext) pluginData {
	includes, err := pluginManager.Includes(page)
	if err != nil {
		grip.Errorf("error getting include html from plugin manager on %v page: %v",
			page, err)
	}

	panels, err := pluginManager.Panels(page)
	if err != nil {
		grip.Errorf("error getting panel html from plugin manager on %v page: %v",
			page, err)
	}

	data, err := pluginManager.UIData(ctx, page)
	if err != nil {
		grip.Errorf("error getting plugin data on %v page: %+v", page, err)
	}

	return pluginData{includes, panels, data}
}
