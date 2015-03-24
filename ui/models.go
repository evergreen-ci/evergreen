package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"10gen.com/mci/plugin"
	"bytes"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"html/template"
	"labix.org/v2/mgo/bson"
	"sort"
	"time"
)

type timelineData struct {
	TotalVersions int
	Versions      []uiVersion
}

type hostsData struct {
	Hosts []uiHost
}

type buildsData struct {
	BuildVariants []string
	TotalBuilds   int
	Builds        []uiBuild
}

type pluginData struct {
	Includes []template.HTML
	Panels   plugin.PanelLayout
	Data     map[string]interface{}
}

type tasksData struct {
	BuildVariants []string
	TotalTasks    int
	Tasks         []uiTask
}

type historiesData struct {
	TaskTypes []string
}

type taskHistoryData struct {
	BuildVariants []string
	Tasks         []uiTask
}

type uiVersion struct {
	Version     model.Version
	Builds      []uiBuild
	PatchInfo   *uiPatch `json:",omitempty"`
	ActiveTasks int
	RepoOwner   string `json:"repo_owner"`
	Repo        string `json:"repo_name"`
}

type uiPatch struct {
	Patch       model.Patch
	StatusDiffs interface{}

	// for linking to other pages
	BaseVersionId string
	BaseBuildId   string
	BaseTaskId    string
}

type uiHost struct {
	Host        host.Host
	RunningTask *model.Task
}

type uiBuild struct {
	Build       model.Build
	Version     model.Version
	PatchInfo   *uiPatch `json:",omitempty"`
	Tasks       []uiTask
	Elapsed     time.Duration
	CurrentTime int64
	RepoOwner   string `json:"repo_owner"`
	Repo        string `json:"repo_name"`
}

type uiTask struct {
	Task          model.Task
	Gitspec       string
	BuildDisplay  string
	TaskLog       []model.LogMessage
	NextTasks     []model.Task
	PreviousTasks []model.Task
	Elapsed       time.Duration
	StartTime     int64
}

// implementation of sort.Interface, to allow uitasks to be sorted
type SortableUiTaskSlice struct {
	tasks []uiTask
}

func (suts *SortableUiTaskSlice) Len() int {
	return len(suts.tasks)
}

func (suts *SortableUiTaskSlice) Less(i, j int) bool {

	taskOne := suts.tasks[i]
	taskTwo := suts.tasks[j]

	displayNameOne := taskOne.Task.DisplayName
	displayNameTwo := taskTwo.Task.DisplayName

	if displayNameOne == mci.CompileStage {
		return true
	}
	if displayNameTwo == mci.CompileStage {
		return false
	}
	if displayNameOne == mci.PushStage {
		return false
	}
	if displayNameTwo == mci.PushStage {
		return true
	}

	if bytes.Compare([]byte(displayNameOne), []byte(displayNameTwo)) == -1 {
		return true
	}
	if bytes.Compare([]byte(displayNameOne), []byte(displayNameTwo)) == 1 {
		return false
	}
	return false
}

func (suts *SortableUiTaskSlice) Swap(i, j int) {
	suts.tasks[i], suts.tasks[j] = suts.tasks[j], suts.tasks[i]
}

func sortUiTasks(tasks []uiTask) []uiTask {
	suts := &SortableUiTaskSlice{tasks}
	sort.Sort(suts)
	return suts.tasks
}

func PopulateUIVersion(version *model.Version) (*uiVersion, error) {
	buildIds := version.BuildIds
	dbBuilds, err := model.FindAllBuilds(bson.M{"_id": bson.M{"$in": buildIds}}, bson.M{},
		db.NoSort, db.NoSkip, db.NoLimit)
	if err != nil {
		return nil, err
	}

	buildsMap := make(map[string]model.Build)
	for _, dbBuild := range dbBuilds {
		buildsMap[dbBuild.Id] = dbBuild
	}

	uiBuilds := make([]uiBuild, len(dbBuilds))
	for buildIdx, buildId := range buildIds {
		build := buildsMap[buildId]
		buildAsUI := uiBuild{Build: build}

		//Use the build's task cache, instead of querying for each individual task.
		uiTasks := make([]uiTask, len(build.Tasks))
		for taskIdx, task := range build.Tasks {
			uiTasks[taskIdx] = uiTask{Task: model.Task{Id: task.Id, Status: task.Status, DisplayName: task.DisplayName}}
		}
		uiTasks = sortUiTasks(uiTasks)

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
	totalVersions, err := model.TotalVersions(bson.M{"branch": projectName})
	if err != nil {
		return nil, err
	}
	data.TotalVersions = totalVersions

	// get the most recent versions, to display in their entirety on the page
	versionsFromDB, err := model.FindAllVersions(
		bson.M{
			model.VersionRequesterKey: requester,
			model.VersionProjectKey:   projectName,
		},
		bson.M{model.VersionConfigKey: 0},
		[]string{"-" + model.VersionRevisionOrderNumberKey},
		versionsToSkip,
		versionsPerPage,
	)
	if err != nil {
		return nil, err
	}

	// create the necessary uiVersion struct for each version
	uiVersions := make([]uiVersion, len(versionsFromDB))
	for versionIdx, version := range versionsFromDB {
		versionAsUI := uiVersion{Version: version}
		uiVersions[versionIdx] = versionAsUI

		buildIds := version.BuildIds
		dbBuilds, err := model.FindAllBuilds(bson.M{"_id": bson.M{"$in": buildIds}}, bson.M{},
			db.NoSort, db.NoSkip, db.NoLimit)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Ids: %v", buildIds)
		}

		buildsMap := make(map[string]model.Build)
		for _, dbBuild := range dbBuilds {
			buildsMap[dbBuild.Id] = dbBuild
		}

		uiBuilds := make([]uiBuild, len(dbBuilds))
		for buildIdx, buildId := range buildIds {
			build := buildsMap[buildId]
			buildAsUI := uiBuild{Build: build}

			//Use the build's task cache, instead of querying for each individual task.
			uiTasks := make([]uiTask, len(build.Tasks))
			for taskIdx, task := range build.Tasks {
				uiTasks[taskIdx] = uiTask{Task: model.Task{Id: task.Id, Status: task.Status, DisplayName: task.DisplayName}}
			}
			uiTasks = sortUiTasks(uiTasks)

			buildAsUI.Tasks = uiTasks
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
func getBuildVariantHistory(buildId string, before int, after int) ([]model.Build, error) {
	build, err := model.FindBuild(buildId)
	if err != nil {
		return nil, err
	}
	if build == nil {
		return nil, fmt.Errorf("no build with id %v", buildId)
	}

	lessRecentBuilds, err := model.FindAllBuilds(
		bson.M{
			model.BuildProjectKey:             build.Project,
			model.BuildBuildVariantKey:        build.BuildVariant,
			model.BuildRequesterKey:           mci.RepotrackerVersionRequester,
			model.BuildRevisionOrderNumberKey: bson.M{"$lt": build.RevisionOrderNumber},
		},
		bson.M{
			model.BuildIdKey:        1,
			model.BuildTasksKey:     1,
			model.BuildStatusKey:    1,
			model.BuildVersionKey:   1,
			model.BuildActivatedKey: 1,
		},
		[]string{fmt.Sprintf("-%v", model.BuildRevisionOrderNumberKey)}, 0, before)

	if err != nil {
		return nil, err
	}

	moreRecentBuilds, err := model.FindAllBuilds(
		bson.M{
			model.BuildProjectKey:             build.Project,
			model.BuildBuildVariantKey:        build.BuildVariant,
			model.BuildRequesterKey:           mci.RepotrackerVersionRequester,
			model.BuildRevisionOrderNumberKey: bson.M{"$gte": build.RevisionOrderNumber},
		},
		bson.M{
			model.BuildIdKey:        1,
			model.BuildTasksKey:     1,
			model.BuildStatusKey:    1,
			model.BuildVersionKey:   1,
			model.BuildActivatedKey: 1,
		},
		[]string{model.BuildRevisionOrderNumberKey}, 0, after)

	if err != nil {
		return nil, err
	}
	builds := make([]model.Build, 0, len(lessRecentBuilds)+len(moreRecentBuilds))

	for i := len(moreRecentBuilds); i > 0; i-- {
		builds = append(builds, moreRecentBuilds[i-1])
	}
	builds = append(builds, lessRecentBuilds...)
	return builds, nil
}

// Given build id, get last successful build before this one
func getBuildVariantHistoryLastSuccess(buildId string) (*model.Build, error) {
	build, err := model.FindBuild(buildId)
	if err != nil {
		return nil, err
	}

	if build.Status == mci.BuildSucceeded {
		return build, nil
	}

	return build.GetPriorBuildWithStatuses([]string{mci.BuildSucceeded})
}

func getVersionHistory(versionId string, N int) ([]model.Version, error) {
	version, err := model.FindVersion(versionId)
	if err != nil {
		return nil, err
	} else if version == nil {
		return nil, fmt.Errorf("Version '%v' not found", versionId)
	}

	// Versions in the same push event, assuming that no two push events happen at the exact same time
	// Never want more than 2N+1 versions, so make sure we add a limit
	siblingVersions, err := model.FindAllVersions(
		bson.M{
			"order":  version.RevisionOrderNumber,
			"r":      mci.RepotrackerVersionRequester,
			"branch": version.Project,
		},
		bson.M{model.VersionConfigKey: 0},
		[]string{"order"}, 0, 2*N+1)

	if err != nil {
		return nil, err
	}

	versionIndex := -1
	for i := 0; i < len(siblingVersions); i++ {
		if siblingVersions[i].Id == version.Id {
			versionIndex = i
		}
	}

	numSiblings := len(siblingVersions) - 1
	versions := siblingVersions

	if versionIndex < N {
		// There are less than N later versions from the same push event
		// N subsequent versions plus the specified one
		subsequentVersions, err := model.FindAllVersions(
			bson.M{
				"order":  bson.M{"$gt": version.RevisionOrderNumber},
				"r":      mci.RepotrackerVersionRequester,
				"branch": version.Project,
			},
			bson.M{model.VersionConfigKey: 0}, []string{"order"}, 0, N-versionIndex)
		if err != nil {
			return nil, err
		}

		// Reverse the second array so we have the versions ordered "newest one first"
		for i := 0; i < len(subsequentVersions)/2; i++ {
			subsequentVersions[i], subsequentVersions[len(subsequentVersions)-1-i] = subsequentVersions[len(subsequentVersions)-1-i], subsequentVersions[i]
		}

		versions = append(subsequentVersions, versions...)
	}

	if numSiblings-versionIndex < N {
		previousVersions, err := model.FindAllVersions(
			bson.M{
				"order":  bson.M{"$lt": version.RevisionOrderNumber},
				"r":      mci.RepotrackerVersionRequester,
				"branch": version.Project,
			},
			bson.M{model.VersionConfigKey: 0}, []string{"-order"}, 0, N)
		if err != nil {
			return nil, err
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
		dbHosts, err = host.Find(host.ByUserWithRunningStatus(mci.MCIUser))
	}

	if err != nil {
		return nil, err
	}

	// convert the hosts to the ui models
	uiHosts := make([]uiHost, len(dbHosts))
	for idx, dbHost := range dbHosts {
		host := uiHost{
			Host:        dbHost,
			RunningTask: nil,
		}

		uiHosts[idx] = host
		// get the task running on this host
		task, err := model.FindTask(dbHost.RunningTask)
		if err != nil {
			return nil, err
		}
		if task != nil {
			uiHosts[idx].RunningTask = task
		}
	}
	data.Hosts = uiHosts
	return data, nil
}

func getHostData(hostId string) (*uiHost, error) {
	hostAsUI := &uiHost{}
	dbHost, err := host.FindOne(host.ById(hostId))
	if err != nil {
		return nil, err
	}
	if dbHost == nil {
		return nil, fmt.Errorf("Could not find host")
	}
	hostAsUI.Host = *dbHost
	return hostAsUI, nil
}

// getPluginDataAndHTML returns all data needed to properly render plugins
// for a page. It logs errors but does not return them, as plugin errors
// cannot stop the rendering of the rest of the page
func getPluginDataAndHTML(pluginManager plugin.PanelManager, page plugin.PageScope, ctx plugin.UIContext) pluginData {
	includes, err := pluginManager.Includes(page)
	if err != nil {
		mci.Logger.Errorf(
			slogger.ERROR, "error getting include html from plugin manager on %v page: %v",
			page, err)
	}

	panels, err := pluginManager.Panels(page)
	if err != nil {
		mci.Logger.Errorf(
			slogger.ERROR, "error getting panel html from plugin manager on %v page: %v",
			page, err)
	}

	data, err := pluginManager.UIData(ctx, page)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "error getting plugin data on %v page: %v",
			page, err)
	}

	return pluginData{includes, panels, data}
}
