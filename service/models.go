package service

import (
	"fmt"
	"html/template"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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
	Version     version.Version
	Builds      []uiBuild
	PatchInfo   *uiPatch `json:",omitempty"`
	ActiveTasks int
	RepoOwner   string `json:"repo_owner"`
	Repo        string `json:"repo_name"`
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
}

type uiBuild struct {
	Build       build.Build
	Version     version.Version
	PatchInfo   *uiPatch `json:",omitempty"`
	Tasks       []uiTask
	Elapsed     time.Duration
	CurrentTime int64
	RepoOwner   string `json:"repo_owner"`
	Repo        string `json:"repo_name"`
}

type uiTask struct {
	Task          task.Task
	Gitspec       string
	BuildDisplay  string
	TaskLog       []model.LogMessage
	NextTasks     []task.Task
	PreviousTasks []task.Task
	Elapsed       time.Duration
	StartTime     int64
}

func PopulateUIVersion(version *version.Version) (*uiVersion, error) {
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
	for buildIdx, buildId := range buildIds {
		build := buildsMap[buildId]
		buildAsUI := uiBuild{Build: build}

		//Use the build's task cache, instead of querying for each individual task.
		uiTasks := make([]uiTask, len(build.Tasks))
		for i, t := range build.Tasks {
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

func getTimelineData(projectName, requester string, versionsToSkip, versionsPerPage int) (*timelineData, error) {
	data := &timelineData{}

	// get the total number of versions in the database (used for pagination)
	totalVersions, err := version.Count(version.ByProjectId(projectName))
	if err != nil {
		return nil, err
	}
	data.TotalVersions = totalVersions

	q := version.ByMostRecentForRequester(projectName, requester).WithoutFields(version.ConfigKey).
		Skip(versionsToSkip * versionsPerPage).Limit(versionsPerPage)

	// get the most recent versions, to display in their entirety on the page
	versionsFromDB, err := version.Find(q)
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
			build := buildsMap[buildId]
			buildAsUI := uiBuild{Build: build}
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

func getVersionHistory(versionId string, N int) ([]version.Version, error) {
	v, err := version.FindOne(version.ById(versionId))
	if err != nil {
		return nil, errors.WithStack(err)
	} else if v == nil {
		return nil, errors.Errorf("Version '%v' not found", versionId)
	}

	// Versions in the same push event, assuming that no two push events happen at the exact same time
	// Never want more than 2N+1 versions, so make sure we add a limit

	siblingVersions, err := version.Find(db.Query(
		bson.M{
			version.RevisionOrderNumberKey: v.RevisionOrderNumber,
			version.RequesterKey:           evergreen.RepotrackerVersionRequester,
			version.IdentifierKey:          v.Identifier,
		}).WithoutFields(version.ConfigKey).Sort([]string{version.RevisionOrderNumberKey}).Limit(2*N + 1))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	versionIndex := -1
	for i := 0; i < len(siblingVersions); i++ {
		if siblingVersions[i].Id == v.Id {
			versionIndex = i
		}
	}

	numSiblings := len(siblingVersions) - 1
	versions := siblingVersions

	if versionIndex < N {
		// There are less than N later versions from the same push event
		// N subsequent versions plus the specified one
		subsequentVersions, err := version.Find(
			//TODO encapsulate this query in version pkg
			db.Query(bson.M{
				version.RevisionOrderNumberKey: bson.M{"$gt": v.RevisionOrderNumber},
				version.RequesterKey:           evergreen.RepotrackerVersionRequester,
				version.IdentifierKey:          v.Identifier,
			}).WithoutFields(version.ConfigKey).Sort([]string{version.RevisionOrderNumberKey}).Limit(N - versionIndex))
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// Reverse the second array so we have the versions ordered "newest one first"
		for i := 0; i < len(subsequentVersions)/2; i++ {
			subsequentVersions[i], subsequentVersions[len(subsequentVersions)-1-i] = subsequentVersions[len(subsequentVersions)-1-i], subsequentVersions[i]
		}

		versions = append(subsequentVersions, versions...)
	}

	if numSiblings-versionIndex < N {
		previousVersions, err := version.Find(db.Query(bson.M{
			version.RevisionOrderNumberKey: bson.M{"$lt": v.RevisionOrderNumber},
			version.RequesterKey:           evergreen.RepotrackerVersionRequester,
			version.IdentifierKey:          v.Identifier,
		}).WithoutFields(version.ConfigKey).Sort([]string{fmt.Sprintf("-%v", version.RevisionOrderNumberKey)}).Limit(N))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		versions = append(versions, previousVersions...)
	}

	return versions, nil
}

func getHostsData(includeSpawnedHosts bool) (*hostsData, error) {
	data := &hostsData{}

	// get all of the hosts
	var dbHosts []host.Host
	var err error
	if includeSpawnedHosts {
		dbHosts, err = host.Find(host.IsRunning)
	} else {
		dbHosts, err = host.Find(host.ByUserWithRunningStatus(evergreen.User))
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

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
		if dbHost.RunningTask != "" {
			task, err := task.FindOne(task.ById(dbHost.RunningTask))
			if err != nil {
				return nil, errors.WithStack(err)
			}
			grip.ErrorWhenf(task == nil, "Hosts page could not find task %s for host %s",
				dbHost.RunningTask, dbHost.Id)
			uiHosts[idx].RunningTask = task
		}
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
