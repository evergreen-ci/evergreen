package service

import (
	"context"
	"html/template"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type hostsData struct {
	Hosts []uiHost
}

type pluginData struct {
	Includes []template.HTML
	Panels   plugin.PanelLayout
	Data     map[string]any
}

type uiVersion struct {
	Version      model.Version
	Builds       []uiBuild
	PatchInfo    *uiPatch `json:",omitempty"`
	ActiveTasks  int
	RepoOwner    string          `json:"repo_owner"`
	Repo         string          `json:"repo_name"`
	UpstreamData *uiUpstreamData `json:"upstream,omitempty"`
	TimeTaken    time.Duration   `json:"time_taken"`
	Makespan     time.Duration   `json:"makespan"`
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

type uiHost struct {
	Host        host.Host
	RunningTask *task.Task
	IdleTime    float64 // idle time in seconds
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

// getBuildVariantHistory returns a slice of builds that surround a given build.
// As many as 'before' builds (less recent builds) plus as many as 'after' builds
// (more recent builds) are returned.
func getBuildVariantHistory(ctx context.Context, buildId string, before int, after int) ([]build.Build, error) {
	b, err := build.FindOne(ctx, build.ById(buildId))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if b == nil {
		return nil, errors.Errorf("no build with id %v", buildId)
	}

	lessRecentBuilds, err := build.Find(ctx,
		build.ByBeforeRevision(b.Project, b.BuildVariant, b.RevisionOrderNumber).
			WithFields(build.IdKey, build.TasksKey, build.StatusKey, build.VersionKey, build.ActivatedKey).
			Limit(before))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	moreRecentBuilds, err := build.Find(ctx,
		build.ByAfterRevision(b.Project, b.BuildVariant, b.RevisionOrderNumber).
			WithFields(build.IdKey, build.TasksKey, build.StatusKey, build.VersionKey, build.ActivatedKey).
			Limit(after))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	builds := make([]build.Build, 0, len(lessRecentBuilds)+len(moreRecentBuilds))
	for i := len(moreRecentBuilds); i > 0; i-- {
		builds = append(builds, moreRecentBuilds[i-1])
	}
	builds = append(builds, lessRecentBuilds...)
	return builds, nil
}

// Given build id, get last successful build before this one
func getBuildVariantHistoryLastSuccess(ctx context.Context, buildId string) (*build.Build, error) {
	b, err := build.FindOne(ctx, build.ById(buildId))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if b.Status == evergreen.BuildSucceeded {
		return b, nil
	}
	b, err = b.PreviousSuccessful(ctx)
	return b, errors.WithStack(err)
}

func getHostsData(ctx context.Context, includeSpawnedHosts bool) (*hostsData, error) {
	dbHosts, err := host.FindRunningHosts(ctx, includeSpawnedHosts)
	if err != nil {
		return nil, errors.Wrap(err, "problem finding hosts")
	}

	data := &hostsData{}

	// convert the hosts to the ui models
	uiHosts := make([]uiHost, len(dbHosts))

	for idx, dbHost := range dbHosts {
		// we only need the distro id for the hosts page
		dbHost.Distro = distro.Distro{Id: dbHost.Distro.Id}
		host := uiHost{
			Host:        dbHost,
			RunningTask: nil,
		}

		uiHosts[idx] = host
		// get the task running on this host
		if dbHost.RunningTaskFull != nil {
			uiHosts[idx].RunningTask = dbHost.RunningTaskFull
		}
		uiHosts[idx].IdleTime = host.Host.IdleTime().Seconds()
	}
	data.Hosts = uiHosts
	return data, nil
}

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
