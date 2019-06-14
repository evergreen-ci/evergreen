package service

import (
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

type timelineData struct {
	TotalVersions int
	Versions      []uiVersion
}

type hostsData struct {
	Hosts []uiHost
}

type pluginData struct {
	Includes []template.HTML
	Panels   plugin.PanelLayout
	Data     map[string]interface{}
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
	StatusDiffs interface{}

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

func PopulateUIVersion(version *model.Version) (*uiVersion, error) {
	buildIds := version.BuildIds
	dbBuilds, err := build.Find(build.ByIds(buildIds))
	if err != nil {
		return nil, err
	}

	buildsMap := make(map[string]build.Build)
	for _, dbBuild := range dbBuilds {
		buildsMap[dbBuild.Id] = dbBuild
	}

	uiBuilds := make([]uiBuild, len(dbBuilds))
	for buildIdx, build := range dbBuilds {
		b := buildsMap[build.Id]
		buildAsUI := uiBuild{Build: b}

		//Use the build's task cache, instead of querying for each individual task.
		uiTasks := make([]uiTask, len(b.Tasks))
		for i, t := range b.Tasks {
			uiTasks[i] = uiTask{
				Task: task.Task{
					Id:          t.Id,
					Status:      t.Status,
					Details:     t.StatusDetails,
					DisplayName: t.DisplayName,
				},
			}
		}

		buildAsUI.Tasks = uiTasks
		uiBuilds[buildIdx] = buildAsUI
	}
	return &uiVersion{Version: (*version), Builds: uiBuilds}, nil
}

///////////////////////////////////////////////////////////////////////////
//// Functions to create and populate the models
///////////////////////////////////////////////////////////////////////////

func getTimelineData(projectName string, versionsToSkip, versionsPerPage int) (*timelineData, error) {
	data := &timelineData{}

	// get the total number of versions in the database (used for pagination)
	totalVersions, err := model.VersionCount(model.VersionByProjectId(projectName))
	if err != nil {
		return nil, err
	}
	data.TotalVersions = totalVersions

	q := model.VersionByMostRecentSystemRequester(projectName).WithoutFields(model.VersionConfigKey).
		Skip(versionsToSkip * versionsPerPage).Limit(versionsPerPage)

	// get the most recent versions, to display in their entirety on the page
	versionsFromDB, err := model.VersionFind(q)
	if err != nil {
		return nil, err
	}

	// create the necessary uiVersion struct for each version
	uiVersions := make([]uiVersion, len(versionsFromDB))
	for versionIdx, version := range versionsFromDB {
		versionAsUI := uiVersion{Version: version}
		uiVersions[versionIdx] = versionAsUI

		buildIds := version.BuildIds
		dbBuilds, err := build.Find(build.ByIds(buildIds))
		grip.ErrorWhenln(err != nil, "Ids:", buildIds)

		buildsMap := make(map[string]build.Build)
		for _, dbBuild := range dbBuilds {
			buildsMap[dbBuild.Id] = dbBuild
		}

		uiBuilds := make([]uiBuild, len(dbBuilds))
		for buildIdx, buildId := range buildIds {
			b := buildsMap[buildId]
			buildAsUI := uiBuild{Build: b}
			uiBuilds[buildIdx] = buildAsUI
		}
		versionAsUI.Builds = uiBuilds
		uiVersions[versionIdx] = versionAsUI
	}

	data.Versions = uiVersions
	return data, nil
}

// getBuildVariantHistory returns a slice of builds that surround a given build.
// As many as 'before' builds (less recent builds) plus as many as 'after' builds
// (more recent builds) are returned.
func getBuildVariantHistory(buildId string, before int, after int) ([]build.Build, error) {
	b, err := build.FindOne(build.ById(buildId))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if b == nil {
		return nil, errors.Errorf("no build with id %v", buildId)
	}

	lessRecentBuilds, err := build.Find(
		build.ByBeforeRevision(b.Project, b.BuildVariant, b.RevisionOrderNumber).
			WithFields(build.IdKey, build.TasksKey, build.StatusKey, build.VersionKey, build.ActivatedKey).
			Limit(before))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	moreRecentBuilds, err := build.Find(
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
func getBuildVariantHistoryLastSuccess(buildId string) (*build.Build, error) {
	b, err := build.FindOne(build.ById(buildId))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if b.Status == evergreen.BuildSucceeded {
		return b, nil
	}
	b, err = b.PreviousSuccessful()
	return b, errors.WithStack(err)
}

func getHostsData(includeSpawnedHosts bool) (*hostsData, error) {
	dbHosts, err := host.FindRunningHosts(includeSpawnedHosts)
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
