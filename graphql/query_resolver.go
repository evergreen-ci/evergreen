package graphql

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/plank"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// BbGetCreatedTickets is the resolver for the bbGetCreatedTickets field.
func (r *queryResolver) BbGetCreatedTickets(ctx context.Context, taskID string) ([]*thirdparty.JiraTicket, error) {
	createdTickets, err := bbGetCreatedTicketsPointers(ctx, taskID)
	if err != nil {
		return nil, err
	}

	return createdTickets, nil
}

// BuildBaron is the resolver for the buildBaron field.
func (r *queryResolver) BuildBaron(ctx context.Context, taskID string, execution int) (*BuildBaron, error) {
	execString := strconv.Itoa(execution)

	searchReturnInfo, bbConfig, err := model.GetSearchReturnInfo(ctx, taskID, execString)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &BuildBaron{
		SearchReturnInfo:        searchReturnInfo,
		BuildBaronConfigured:    bbConfig.ProjectFound && bbConfig.SearchConfigured,
		BbTicketCreationDefined: bbConfig.TicketCreationDefined,
	}, nil
}

// AdminEvents is the resolver for the adminEvents field.
func (r *queryResolver) AdminEvents(ctx context.Context, opts AdminEventsInput) (*AdminEventsPayload, error) {
	before := utility.FromTimePtr(opts.Before)
	limit := utility.FromIntPtr(opts.Limit)

	events, err := event.FindLatestAdminEvents(ctx, limit, before)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("retrieving admin events: %s", err.Error()))
	}

	eventLogEntries := []*AdminEvent{}
	for _, e := range events {
		entry, err := makeAdminEvent(ctx, e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		eventLogEntries = append(eventLogEntries, entry)
	}

	return &AdminEventsPayload{
		EventLogEntries: eventLogEntries,
		Count:           len(eventLogEntries),
	}, nil
}

// AdminSettings is the resolver for the adminSettings field.
func (r *queryResolver) AdminSettings(ctx context.Context) (*restModel.APIAdminSettings, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	adminSettings := restModel.NewConfigModel()
	if err = adminSettings.BuildFromService(config); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("building API admin settings from service: %s", err.Error()))
	}
	return adminSettings, nil
}

// AdminTasksToRestart is the resolver for the adminTasksToRestart field.
func (r *queryResolver) AdminTasksToRestart(ctx context.Context, opts model.RestartOptions) (*AdminTasksToRestartPayload, error) {
	env := evergreen.GetEnvironment()
	usr := mustHaveUser(ctx)
	opts.User = usr.Username()

	// Set DryRun = true so that we fetch a list of tasks to restart.
	opts.DryRun = true
	results, err := data.RestartFailedTasks(ctx, env.RemoteQueue(), opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching restart tasks: %s", err.Error()))
	}

	tasksToRestart := []*restModel.APITask{}
	for _, taskID := range results.ItemsRestarted {
		t, err := task.FindOneId(ctx, taskID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
		}
		if t == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
		}
		apiTask := &restModel.APITask{}
		if err = apiTask.BuildFromService(ctx, t, nil); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", taskID, err.Error()))
		}
		tasksToRestart = append(tasksToRestart, apiTask)
	}

	return &AdminTasksToRestartPayload{
		TasksToRestart: tasksToRestart,
	}, nil
}

// AWSRegions is the resolver for the awsRegions field.
func (r *queryResolver) AWSRegions(ctx context.Context) ([]string, error) {
	return evergreen.GetEnvironment().Settings().Providers.AWS.AllowedRegions, nil
}

// ClientConfig is the resolver for the clientConfig field.
func (r *queryResolver) ClientConfig(ctx context.Context) (*restModel.APIClientConfig, error) {
	envClientConfig := evergreen.GetEnvironment().ClientConfig()
	clientConfig := restModel.APIClientConfig{}
	clientConfig.BuildFromService(*envClientConfig)
	return &clientConfig, nil
}

// InstanceTypes is the resolver for the instanceTypes field.
func (r *queryResolver) InstanceTypes(ctx context.Context) ([]string, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	return config.Providers.AWS.AllowedInstanceTypes, nil
}

// SpruceConfig is the resolver for the spruceConfig field.
func (r *queryResolver) SpruceConfig(ctx context.Context) (*restModel.APIAdminSettings, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}

	spruceConfig := restModel.APIAdminSettings{}
	err = spruceConfig.BuildFromService(config)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("building API admin settings from service: %s", err.Error()))
	}
	return &spruceConfig, nil
}

// SubnetAvailabilityZones is the resolver for the subnetAvailabilityZones field.
func (r *queryResolver) SubnetAvailabilityZones(ctx context.Context) ([]string, error) {
	zones := []string{}
	for _, subnet := range evergreen.GetEnvironment().Settings().Providers.AWS.Subnets {
		zones = append(zones, subnet.AZ)
	}
	return zones, nil
}

// Distro is the resolver for the distro field.
func (r *queryResolver) Distro(ctx context.Context, distroID string) (*restModel.APIDistro, error) {
	d, err := distro.FindOneId(ctx, distroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching distro '%s': %s", distroID, err.Error()))
	}
	if d == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("distro '%s' not found", distroID))
	}

	apiDistro := restModel.APIDistro{}
	apiDistro.BuildFromService(*d)
	return &apiDistro, nil
}

// DistroEvents is the resolver for the distroEvents field.
func (r *queryResolver) DistroEvents(ctx context.Context, opts DistroEventsInput) (*DistroEventsPayload, error) {
	before := utility.FromTimePtr(opts.Before)

	limit := 10
	if opts.Limit != nil {
		limit = utility.FromIntPtr(opts.Limit)
	}

	events, err := event.FindLatestPrimaryDistroEvents(ctx, opts.DistroID, limit, before)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("retrieving events for distro '%s': %s", opts.DistroID, err.Error()))
	}

	eventLogEntries := []*DistroEvent{}
	for _, e := range events {
		entry, err := makeDistroEvent(ctx, e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		eventLogEntries = append(eventLogEntries, entry)
	}

	return &DistroEventsPayload{
		EventLogEntries: eventLogEntries,
		Count:           len(eventLogEntries),
	}, nil
}

// Distros is the resolver for the distros field.
func (r *queryResolver) Distros(ctx context.Context, onlySpawnable bool) ([]*restModel.APIDistro, error) {
	usr := mustHaveUser(ctx)
	apiDistros := []*restModel.APIDistro{}

	var distros []distro.Distro
	if onlySpawnable {
		d, err := distro.Find(ctx, distro.BySpawnAllowed())
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching spawnable distros: %s", err.Error()))
		}
		distros = d
	} else {
		d, err := distro.AllDistros(ctx)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching distros: %s", err.Error()))
		}
		distros = d
	}

	userHasDistroCreatePermission := usr.HasDistroCreatePermission(ctx)

	for _, d := range distros {
		// Omit admin-only distros if user lacks permissions
		if d.AdminOnly && !userHasDistroCreatePermission {
			continue
		}

		apiDistro := restModel.APIDistro{}
		apiDistro.BuildFromService(d)
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

// DistroTaskQueue is the resolver for the distroTaskQueue field.
func (r *queryResolver) DistroTaskQueue(ctx context.Context, distroID string) ([]*restModel.APITaskQueueItem, error) {
	distroQueue, err := model.LoadTaskQueue(ctx, distroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task queue for distro '%s': %s", distroID, err.Error()))
	}
	if distroQueue == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task queue for distro '%s' not found", distroID))
	}

	idToIdentifierMap := map[string]string{}
	taskQueue := []*restModel.APITaskQueueItem{}

	for _, taskQueueItem := range distroQueue.Queue {
		apiTaskQueueItem := restModel.APITaskQueueItem{}

		if _, ok := idToIdentifierMap[taskQueueItem.Project]; !ok {
			identifier, err := model.GetIdentifierForProject(ctx, taskQueueItem.Project)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting identifier for project '%s': %s", taskQueueItem.Project, err.Error()))
			}
			idToIdentifierMap[taskQueueItem.Project] = identifier
		}

		apiTaskQueueItem.BuildFromService(taskQueueItem)
		if identifier := idToIdentifierMap[taskQueueItem.Project]; identifier != "" {
			apiTaskQueueItem.ProjectIdentifier = utility.ToStringPtr(identifier)
		}
		taskQueue = append(taskQueue, &apiTaskQueueItem)
	}

	return taskQueue, nil
}

// Host is the resolver for the host field.
func (r *queryResolver) Host(ctx context.Context, hostID string) (*restModel.APIHost, error) {
	host, err := host.GetHostByIdOrTagWithTask(ctx, hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching host '%s': %s", hostID, err.Error()))
	}
	if host == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", hostID))
	}

	apiHost := &restModel.APIHost{}
	apiHost.BuildFromService(host, host.RunningTaskFull)
	return apiHost, nil
}

// HostEvents is the resolver for the hostEvents field.
func (r *queryResolver) HostEvents(ctx context.Context, hostID string, hostTag *string, limit *int, page *int) (*HostEvents, error) {
	h, err := host.FindOneByIdOrTag(ctx, hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching host '%s': %s", hostID, err.Error()))
	}
	if h == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", hostID))
	}
	hostQueryOpts := event.PaginatedHostEventsOpts{
		ID:      h.Id,
		Tag:     utility.FromStringPtr(hostTag),
		Limit:   utility.FromIntPtr(limit),
		Page:    utility.FromIntPtr(page),
		SortAsc: false,
	}
	events, count, err := event.GetPaginatedHostEvents(ctx, hostQueryOpts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching events for host '%s': %s", hostID, err.Error()))
	}
	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.HostAPIEventLogEntry{}
	for _, e := range events {
		apiEventLog := restModel.HostAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("building APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	hostevents := HostEvents{
		EventLogEntries: apiEventLogPointers,
		Count:           count,
	}
	return &hostevents, nil
}

// Hosts is the resolver for the hosts field.
func (r *queryResolver) Hosts(ctx context.Context, hostID *string, distroID *string, currentTaskID *string, statuses []string, startedBy *string, sortBy *HostSortBy, sortDir *SortDirection, page *int, limit *int) (*HostsResponse, error) {
	hostIDParam := ""
	if hostID != nil {
		hostIDParam = *hostID
	}
	distroParam := ""
	if distroID != nil {
		distroParam = *distroID
	}
	currentTaskParam := ""
	if currentTaskID != nil {
		currentTaskParam = *currentTaskID
	}
	startedByParam := ""
	if startedBy != nil {
		startedByParam = *startedBy
	}
	sorter := host.StatusKey
	if sortBy != nil {
		switch *sortBy {
		case HostSortByCurrentTask:
			sorter = host.RunningTaskKey
		case HostSortByDistro:
			sorter = bsonutil.GetDottedKeyName(host.DistroKey, distro.IdKey)
		case HostSortByElapsed:
			sorter = "task_full.start_time"
		case HostSortByID:
			sorter = host.IdKey
		case HostSortByIdleTime:
			sorter = host.TotalIdleTimeKey
		case HostSortByOwner:
			sorter = host.StartedByKey
		case HostSortByStatus:
			sorter = host.StatusKey
		case HostSortByUptime:
			sorter = host.CreateTimeKey
		default:
			sorter = host.StatusKey
		}

	}
	sortDirParam := 1
	if *sortDir == SortDirectionDesc {
		sortDirParam = -1
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}

	hostsFilterOpts := host.HostsFilterOptions{
		HostID:        hostIDParam,
		DistroID:      distroParam,
		CurrentTaskID: currentTaskParam,
		Statuses:      statuses,
		StartedBy:     startedByParam,
		SortBy:        sorter,
		SortDir:       sortDirParam,
		Page:          pageParam,
		Limit:         limitParam,
	}

	hosts, filteredHostsCount, totalHostsCount, err := host.GetPaginatedRunningHosts(ctx, hostsFilterOpts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching hosts: %s", err.Error()))
	}

	usr := mustHaveUser(ctx)
	apiHosts := []*restModel.APIHost{}
	for _, h := range hosts {
		forbiddenHosts := []string{}
		if !userHasHostPermission(ctx, usr, h.Distro.Id, evergreen.HostsView.Value, h.StartedBy) {
			forbiddenHosts = append(forbiddenHosts, h.Id)
		}
		if len(forbiddenHosts) > 0 {
			grip.Info(message.Fields{
				"message":         "User does not have permission to view hosts",
				"forbidden_hosts": forbiddenHosts,
				"user":            usr.Username(),
				"ticket":          "DEVPROD-5753",
			})
		}
		apiHost := restModel.APIHost{}
		apiHost.BuildFromService(&h, h.RunningTaskFull)
		apiHosts = append(apiHosts, &apiHost)
	}
	return &HostsResponse{
		Hosts:              apiHosts,
		FilteredHostsCount: filteredHostsCount,
		TotalHostsCount:    totalHostsCount,
	}, nil
}

// TaskQueueDistros is the resolver for the taskQueueDistros field.
func (r *queryResolver) TaskQueueDistros(ctx context.Context) ([]*TaskQueueDistro, error) {
	queues, err := model.FindAllTaskQueues(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching all task queues: %s", err.Error()))
	}

	distros := []*TaskQueueDistro{}

	for _, distro := range queues {
		numHosts, err := host.CountHostsCanRunTasks(ctx, distro.Distro)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching associated hosts: %s", err.Error()))
		}
		tqd := TaskQueueDistro{
			ID:        distro.Distro,
			TaskCount: len(distro.Queue),
			HostCount: numHosts,
		}
		distros = append(distros, &tqd)
	}

	// sort distros by task count in descending order
	sort.SliceStable(distros, func(i, j int) bool {
		return distros[i].TaskCount > distros[j].TaskCount
	})

	return distros, nil
}

// Patch is the resolver for the patch field.
func (r *queryResolver) Patch(ctx context.Context, patchID string) (*restModel.APIPatch, error) {
	apiPatch, err := data.FindPatchById(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()))
	}
	if apiPatch == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' not found", patchID))
	}
	return apiPatch, nil
}

// GithubProjectConflicts is the resolver for the githubProjectConflicts field.
func (r *queryResolver) GithubProjectConflicts(ctx context.Context, projectID string) (*model.GithubProjectConflicts, error) {
	pRef, err := model.FindMergedProjectRef(ctx, projectID, "", false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectID))
	}

	conflicts, err := pRef.GetGithubProjectConflicts(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting project conflicts: %s", err.Error()))
	}
	return &conflicts, nil
}

// Project is the resolver for the project field.
func (r *queryResolver) Project(ctx context.Context, projectIdentifier string) (*restModel.APIProjectRef, error) {
	project, err := data.FindProjectById(ctx, projectIdentifier, true, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectIdentifier, err.Error()))
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(ctx, *project)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", projectIdentifier, err.Error()))
	}
	return &apiProjectRef, nil
}

// Projects is the resolver for the projects field.
func (r *queryResolver) Projects(ctx context.Context) ([]*GroupedProjects, error) {
	usr := mustHaveUser(ctx)
	viewableProjectIds, err := usr.GetViewableProjects(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting viewable projects for user '%s': %s", usr.Username(), err.Error()))
	}
	allProjects, err := model.FindMergedEnabledProjectRefsByIds(ctx, viewableProjectIds...)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("getting merged enabled project refs for user '%s': %s", usr.Username(), err.Error()))
	}
	groupedProjects, err := groupProjects(ctx, allProjects, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("grouping projects: %s", err.Error()))
	}
	return groupedProjects, nil
}

// ProjectEvents is the resolver for the projectEvents field.
func (r *queryResolver) ProjectEvents(ctx context.Context, projectIdentifier string, limit *int, before *time.Time) (*ProjectEvents, error) {
	timestamp := time.Now()
	if before != nil {
		timestamp = *before
	}
	events, err := data.GetProjectEventLog(ctx, projectIdentifier, timestamp, utility.FromIntPtr(limit))
	res := &ProjectEvents{
		EventLogEntries: getPointerEventList(events),
		Count:           len(events),
	}
	return res, err
}

// ProjectSettings is the resolver for the projectSettings field.
func (r *queryResolver) ProjectSettings(ctx context.Context, projectIdentifier string) (*restModel.APIProjectSettings, error) {
	projectRef, err := model.FindBranchProjectRef(ctx, projectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectIdentifier, err.Error()))
	}
	if projectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectIdentifier))
	}

	res := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{},
	}
	if err = res.ProjectRef.BuildFromService(ctx, *projectRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", projectIdentifier, err.Error()))
	}
	if !projectRef.UseRepoSettings() {
		// Default values so the UI understands what to do with nil values.
		res.ProjectRef.DefaultUnsetBooleans()
	}
	return res, nil
}

// RepoEvents is the resolver for the repoEvents field.
func (r *queryResolver) RepoEvents(ctx context.Context, repoID string, limit *int, before *time.Time) (*ProjectEvents, error) {
	timestamp := time.Now()
	if before != nil {
		timestamp = *before
	}
	events, err := data.GetEventsById(ctx, repoID, timestamp, utility.FromIntPtr(limit))
	res := &ProjectEvents{
		EventLogEntries: getPointerEventList(events),
		Count:           len(events),
	}
	return res, err
}

// RepoSettings is the resolver for the repoSettings field.
func (r *queryResolver) RepoSettings(ctx context.Context, repoID string) (*restModel.APIProjectSettings, error) {
	repoRef, err := model.FindOneRepoRef(ctx, repoID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching repo '%s': %s", repoID, err.Error()))
	}
	if repoRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("repo '%s' not found", repoID))
	}

	res := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{},
	}
	if err = res.ProjectRef.BuildFromService(ctx, repoRef.ProjectRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting repo '%s' to APIProjectRef: %s", repoID, err.Error()))
	}

	// Default values so the UI understands what to do with nil values.
	res.ProjectRef.DefaultUnsetBooleans()
	return res, nil
}

// ViewableProjectRefs is the resolver for the viewableProjectRefs field.
func (r *queryResolver) ViewableProjectRefs(ctx context.Context) ([]*GroupedProjects, error) {
	usr := mustHaveUser(ctx)
	projectIds, err := usr.GetViewableProjectSettings(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting viewable projects for user '%s': %s", usr.Username(), err.Error()))
	}

	projects, err := model.FindProjectRefsByIds(ctx, projectIds...)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting projects: %s", err.Error()))
	}

	groupedProjects, err := groupProjects(ctx, projects, true)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("grouping projects: %s", err.Error()))
	}
	return groupedProjects, nil
}

// IsRepo is the resolver for the isRepo field.
func (r *queryResolver) IsRepo(ctx context.Context, projectOrRepoID string) (bool, error) {
	repo, err := model.FindOneRepoRef(ctx, projectOrRepoID)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching repo '%s': %s", projectOrRepoID, err.Error()))
	}
	if repo == nil {
		return false, nil
	}
	return true, nil
}

// MyHosts is the resolver for the myHosts field.
func (r *queryResolver) MyHosts(ctx context.Context) ([]*restModel.APIHost, error) {
	usr := mustHaveUser(ctx)
	hosts, err := host.Find(ctx, host.ByUserWithRunningStatus(usr.Username()))
	if err != nil {
		return nil, InternalServerError.Send(ctx,
			fmt.Sprintf("fetching running hosts for user '%s': %s", usr.Username(), err.Error()))
	}
	duration := time.Duration(5) * time.Minute
	timestamp := time.Now().Add(-duration) // within last 5 minutes
	recentlyTerminatedHosts, err := host.Find(ctx, host.ByUserRecentlyTerminated(usr.Username(), timestamp))
	if err != nil {
		return nil, InternalServerError.Send(ctx,
			fmt.Sprintf("fetching recently terminated hosts for user '%s': %s", usr.Username(), err.Error()))
	}
	hosts = append(hosts, recentlyTerminatedHosts...)

	var apiHosts []*restModel.APIHost
	for _, h := range hosts {
		apiHost := restModel.APIHost{}
		apiHost.BuildFromService(&h, nil)
		apiHosts = append(apiHosts, &apiHost)
	}
	return apiHosts, nil
}

// MyVolumes is the resolver for the myVolumes field.
func (r *queryResolver) MyVolumes(ctx context.Context) ([]*restModel.APIVolume, error) {
	usr := mustHaveUser(ctx)
	volumes, err := host.FindSortedVolumesByUser(ctx, usr.Username())
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return getAPIVolumeList(volumes)
}

// LogkeeperBuildMetadata is the resolver for the logkeeperBuildMetadata field.
func (r *queryResolver) LogkeeperBuildMetadata(ctx context.Context, buildID string) (*plank.Build, error) {
	client := plank.NewLogkeeperClient(plank.NewLogkeeperClientOptions{
		BaseURL: evergreen.GetEnvironment().Settings().LoggerConfig.LogkeeperURL,
	})
	build, err := client.GetBuildMetadata(ctx, buildID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching Logkeeper build metadata: %s", err.Error()))
	}
	return &build, nil
}

// Task is the resolver for the task field.
func (r *queryResolver) Task(ctx context.Context, taskID string, execution *int) (*restModel.APITask, error) {
	return getTask(ctx, taskID, execution, r.sc.GetURL())
}

// TaskAllExecutions is the resolver for the taskAllExecutions field.
func (r *queryResolver) TaskAllExecutions(ctx context.Context, taskID string) ([]*restModel.APITask, error) {
	latestTask, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if latestTask == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	allTasks := []*restModel.APITask{}
	for i := 0; i < latestTask.Execution; i++ {
		var dbTask *task.Task
		dbTask, err = task.FindByIdExecution(ctx, taskID, &i)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s' with execution %d: %s", taskID, i, err.Error()))
		}
		if dbTask == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' with execution %d not found", taskID, i))
		}
		var apiTask *restModel.APITask
		apiTask, err = getAPITaskFromTask(ctx, r.sc.GetURL(), *dbTask)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' with execution %d to APITask", taskID, i))
		}
		allTasks = append(allTasks, apiTask)
	}
	apiTask, err := getAPITaskFromTask(ctx, r.sc.GetURL(), *latestTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting latest execution of task '%s' to APITask", taskID))
	}
	allTasks = append(allTasks, apiTask)
	return allTasks, nil
}

// TaskTestSample is the resolver for the taskTestSample field.
func (r *queryResolver) TaskTestSample(ctx context.Context, versionID string, taskIds []string, filters []*TestFilter) ([]*TaskTestResultSample, error) {
	if len(taskIds) == 0 {
		return nil, nil
	}
	dbTasks, err := task.FindAll(ctx, db.Query(task.ByIds(taskIds)))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching tasks '%s': %s", taskIds, err.Error()))
	}
	if len(dbTasks) == 0 {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("tasks '%s' not found", taskIds))
	}

	failingTests := []string{}
	for _, f := range filters {
		failingTests = append(failingTests, f.TestName)
	}

	var allTasks []task.Task
	apiSamples := make([]*TaskTestResultSample, len(dbTasks))
	apiSamplesByTaskID := map[string]*TaskTestResultSample{}
	for i, dbTask := range dbTasks {
		if dbTask.Version != versionID && dbTask.ParentPatchID != versionID {
			return nil, InputValidationError.Send(ctx, fmt.Sprintf("task '%s' does not belong to version '%s'", dbTask.Id, versionID))
		}
		tasks, err := dbTask.GetTestResultsTasks(ctx)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("creating test results task options for task '%s': %s", dbTask.Id, err.Error()))
		}

		apiSamples[i] = &TaskTestResultSample{TaskID: dbTask.Id, Execution: dbTask.Execution}
		for _, o := range tasks {
			apiSamplesByTaskID[o.Id] = apiSamples[i]
		}
		allTasks = append(allTasks, tasks...)
	}

	if len(allTasks) > 0 {
		samples, err := task.GetFailedTestSamples(ctx, evergreen.GetEnvironment(), allTasks, failingTests)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting test results sample: %s", err.Error()))
		}

		for _, sample := range samples {
			apiSample, ok := apiSamplesByTaskID[sample.TaskID]
			if !ok {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("unexpected task '%s' in task test sample result", sample.TaskID))
			}

			apiSample.MatchingFailedTestNames = append(apiSample.MatchingFailedTestNames, sample.MatchingFailedTestNames...)
			apiSample.TotalTestCount += sample.TotalFailedNames
		}
	}

	return apiSamples, nil
}

// CursorSettings is the resolver for the cursorSettings field.
func (r *queryResolver) CursorSettings(ctx context.Context) (*CursorSettings, error) {
	usr := mustHaveUser(ctx)

	sageConfig := &evergreen.SageConfig{}
	if err := sageConfig.Get(ctx); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Sage config: %s", err.Error()))
	}

	sageClient, err := thirdparty.NewSageClient(sageConfig.BaseURL)
	if err != nil {
		// Return a default response indicating the feature is not configured.
		// NewSageClient returns an error when the base URL is empty.
		return &CursorSettings{
			KeyConfigured: false,
			KeyLastFour:   nil,
		}, nil
	}
	defer sageClient.Close()

	result, err := sageClient.GetCursorAPIKeyStatus(ctx, usr.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Cursor API key status: %s", err.Error()))
	}

	return &CursorSettings{
		KeyConfigured: result.HasKey,
		KeyLastFour:   utility.ToStringPtr(result.KeyLastFour),
	}, nil
}

// MyPublicKeys is the resolver for the myPublicKeys field.
func (r *queryResolver) MyPublicKeys(ctx context.Context) ([]*restModel.APIPubKey, error) {
	publicKeys := getMyPublicKeys(ctx)
	return publicKeys, nil
}

// User is the resolver for the user field.
func (r *queryResolver) User(ctx context.Context, userID *string) (*restModel.APIDBUser, error) {
	usr := mustHaveUser(ctx)
	var err error
	if userID != nil {
		usr, err = user.FindOneById(ctx, utility.FromStringPtr(userID))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching user '%s': %s", utility.FromStringPtr(userID), err.Error()))
		}
		if usr == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("user '%s' not found", utility.FromStringPtr(userID)))
		}
	}
	apiUser := restModel.APIDBUser{}
	apiUser.BuildFromService(*usr)
	return &apiUser, nil
}

// UserConfig is the resolver for the userConfig field.
func (r *queryResolver) UserConfig(ctx context.Context) (*UserConfig, error) {
	usr := mustHaveUser(ctx)
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	config := &UserConfig{
		User: usr.Username(),
	}
	if settings != nil {
		config.UIServerHost = settings.Ui.Url
		if !settings.ServiceFlags.JWTTokenForCLIDisabled {
			config.APIServerHost = settings.Api.CorpURL + "/api"
		} else {
			config.APIServerHost = settings.Api.URL + "/api"
		}
		if settings.AuthConfig.OAuth != nil {
			config.OauthIssuer = settings.AuthConfig.OAuth.Issuer
			config.OauthClientID = settings.AuthConfig.OAuth.ClientID
			config.OauthConnectorID = settings.AuthConfig.OAuth.ConnectorID
		}
		if !settings.ServiceFlags.StaticAPIKeysDisabled {
			config.APIKey = usr.GetAPIKey()
		}
	}

	return config, nil
}

// BuildVariantsForTaskName is the resolver for the buildVariantsForTaskName field.
func (r *queryResolver) BuildVariantsForTaskName(ctx context.Context, projectIdentifier string, taskName string) ([]*task.BuildVariantTuple, error) {
	pid, err := model.GetIdForProject(ctx, projectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectIdentifier, err.Error()))
	}
	repo, err := model.FindRepository(ctx, pid)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching repository for project '%s': %s", pid, err.Error()))
	}
	if repo == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("repository for project '%s' not found", pid))
	}
	taskBuildVariants, err := task.FindUniqueBuildVariantNamesByTask(ctx, pid, taskName, repo.RevisionOrderNumber)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting build variants for task '%s': %s", taskName, err.Error()))
	}
	return taskBuildVariants, nil
}

// MainlineCommits is the resolver for the mainlineCommits field.
func (r *queryResolver) MainlineCommits(ctx context.Context, options MainlineCommitsOptions, buildVariantOptions *BuildVariantOptions) (*MainlineCommits, error) {
	projectId, err := model.GetIdForProject(ctx, options.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", options.ProjectIdentifier, err.Error()))
	}
	limit := model.DefaultMainlineCommitVersionLimit
	if utility.FromIntPtr(options.Limit) != 0 {
		limit = utility.FromIntPtr(options.Limit)
	}
	requesters := options.Requesters
	if len(requesters) == 0 {
		requesters = evergreen.SystemVersionRequesterTypes
	}

	skipOrderNumber := utility.FromIntPtr(options.SkipOrderNumber)
	revision := utility.FromStringPtr(options.Revision)

	if options.SkipOrderNumber == nil && options.Revision != nil {
		order, err := model.GetOffsetVersionOrderByRevision(ctx, revision, projectId, limit)
		if err != nil {
			graphql.AddError(ctx, PartialError.Send(ctx, err.Error()))
		} else {
			skipOrderNumber = order
		}
	}

	opts := model.MainlineCommitVersionOptions{
		Limit:           limit,
		SkipOrderNumber: skipOrderNumber,
		Requesters:      requesters,
	}

	versions, err := model.GetMainlineCommitVersionsWithOptions(ctx, projectId, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting activated versions: %s", err.Error()))
	}

	var mainlineCommits MainlineCommits

	// We only want to return the PrevPageOrderNumber if a user is not on the first page.
	if skipOrderNumber != 0 {
		prevPageCommit, err := model.GetPreviousPageCommitOrderNumber(ctx, projectId, skipOrderNumber, limit, requesters)

		if err != nil {
			// This shouldn't really happen, but if it does, we should return an error and log it
			grip.Warning(message.WrapError(err, message.Fields{
				"message":    "Error getting most recent version",
				"project_id": projectId,
			}))
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting most recent mainline commit: %s", err.Error()))
		}

		if prevPageCommit != nil {
			mainlineCommits.PrevPageOrderNumber = prevPageCommit
		}
	}

	matchingVersionCount := 0
	versionsCheckedCount := 0
	hasFilters := isPopulated(buildVariantOptions) && utility.FromBoolPtr(options.ShouldCollapse)

	// We will loop through each version returned from GetMainlineCommitVersionsWithOptions and see if there is a commit
	// that matches the filter parameters (if any).
	// If there is a match, we will add it to the array of versions to be returned to the user.
	// If there are not enough matches to satisfy our limit, we will call GetMainlineCommitVersionsWithOptions again
	// with the next order number to check and repeat the process.
	for matchingVersionCount < limit {
		// If we no longer have any more versions to check, break out of the loop.
		if len(versions) == 0 {
			break
		}

		// If we have checked more versions than the MaxMainlineCommitVersionLimit, break out of the loop.
		if versionsCheckedCount >= model.MaxMainlineCommitVersionLimit {
			if matchingVersionCount == 0 {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("matching version not found in %d most recent versions", model.MaxMainlineCommitVersionLimit))
			}
			break
		}

		var versionsMatchingTasksMap map[string]bool
		if hasFilters {
			activeVersions := utility.FilterSlice(versions, func(s model.Version) bool { return utility.FromBoolPtr(s.Activated) })
			opts := task.HasMatchingTasksOptions{
				TaskNames:                  buildVariantOptions.Tasks,
				Variants:                   buildVariantOptions.Variants,
				Statuses:                   getValidTaskStatusesFilter(buildVariantOptions.Statuses),
				IncludeNeverActivatedTasks: true,
			}
			versionsMatchingTasksMap, err = concurrentlyBuildVersionsMatchingTasksMap(ctx, activeVersions, opts)
			if err != nil {
				return nil, InternalServerError.Send(ctx, err.Error())
			}
		}

		// Loop through the current versions to check for matching versions.
		for _, v := range versions {
			mainlineCommitVersion := MainlineCommitVersion{}
			apiVersion := restModel.APIVersion{}
			apiVersion.BuildFromService(ctx, v)
			versionsCheckedCount++

			if !utility.FromBoolPtr(v.Activated) {
				collapseCommit(ctx, mainlineCommits, &mainlineCommitVersion, apiVersion)
			} else if hasFilters && !versionsMatchingTasksMap[v.Id] {
				collapseCommit(ctx, mainlineCommits, &mainlineCommitVersion, apiVersion)
			} else {
				matchingVersionCount++
				mainlineCommits.NextPageOrderNumber = utility.ToIntPtr(v.RevisionOrderNumber)
				mainlineCommitVersion.Version = &apiVersion
			}

			// Only add a mainlineCommit if a new one was added and it's not a modified existing RolledUpVersion.
			if mainlineCommitVersion.Version != nil || mainlineCommitVersion.RolledUpVersions != nil {
				mainlineCommits.Versions = append(mainlineCommits.Versions, &mainlineCommitVersion)
			}

			if matchingVersionCount >= limit {
				break
			}
		}

		// If we don't have enough matching versions, fetch more versions to check.
		if matchingVersionCount < limit {
			skipOrderNumber := versions[len(versions)-1].RevisionOrderNumber
			opts := model.MainlineCommitVersionOptions{
				Limit:           limit,
				SkipOrderNumber: skipOrderNumber,
				Requesters:      requesters,
			}
			versions, err = model.GetMainlineCommitVersionsWithOptions(ctx, projectId, opts)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching more mainline commit versions: %s", err.Error()))
			}
		}
	}

	return &mainlineCommits, nil
}

// TaskNamesForBuildVariant is the resolver for the taskNamesForBuildVariant field.
func (r *queryResolver) TaskNamesForBuildVariant(ctx context.Context, projectIdentifier string, buildVariant string) ([]string, error) {
	pid, err := model.GetIdForProject(ctx, projectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectIdentifier, err.Error()))
	}
	repo, err := model.FindRepository(ctx, pid)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching repository for project '%s': %s", pid, err.Error()))
	}
	if repo == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("repository for project '%s' not found", pid))
	}
	buildVariantTasks, err := task.FindTaskNamesByBuildVariant(ctx, pid, buildVariant, repo.RevisionOrderNumber)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for build variant '%s': %s", buildVariant, err.Error()))
	}
	if buildVariantTasks == nil {
		return []string{}, nil
	}
	return buildVariantTasks, nil
}

// Waterfall is the resolver for the waterfall field.
func (r *queryResolver) Waterfall(ctx context.Context, options WaterfallOptions) (*Waterfall, error) {
	projectId, err := model.GetIdForProject(ctx, options.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", options.ProjectIdentifier, err.Error()))
	}
	limit := model.DefaultWaterfallVersionLimit
	if limitOpt := utility.FromIntPtr(options.Limit); limitOpt != 0 {
		if limitOpt > model.MaxWaterfallVersionLimit {
			return nil, InputValidationError.Send(ctx, fmt.Sprintf("limit exceeds max limit of %d", model.MaxWaterfallVersionLimit))
		}
		limit = limitOpt
	}

	requesters := options.Requesters
	if len(requesters) == 0 {
		requesters = evergreen.SystemVersionRequesterTypes
	}

	maxOrderOpt := utility.FromIntPtr(options.MaxOrder)
	minOrderOpt := utility.FromIntPtr(options.MinOrder)

	if options.Revision != nil {
		revision := utility.FromStringPtr(options.Revision)
		order, err := model.GetOffsetVersionOrderByRevision(ctx, revision, projectId, limit)
		if err != nil {
			graphql.AddError(ctx, PartialError.Send(ctx, err.Error()))
		} else {
			maxOrderOpt = order
		}
	} else if options.Date != nil {
		date := utility.FromTimePtr(options.Date)
		order, err := model.GetOffsetVersionOrderByDate(ctx, date, projectId)
		if err != nil {
			graphql.AddError(ctx, PartialError.Send(ctx, err.Error()))
		} else {
			maxOrderOpt = order
		}
	}

	opts := model.WaterfallOptions{
		Limit:                limit,
		MaxOrder:             maxOrderOpt,
		MinOrder:             minOrderOpt,
		OmitInactiveBuilds:   utility.FromBoolPtr(options.OmitInactiveBuilds),
		Requesters:           requesters,
		Statuses:             utility.FilterSlice(options.Statuses, func(s string) bool { return s != "" }),
		Tasks:                utility.FilterSlice(options.Tasks, func(s string) bool { return s != "" }),
		TaskCaseSensitive:    utility.FromBoolTPtr(options.TaskCaseSensitive), // Default to true for performance reasons.
		Variants:             utility.FilterSlice(options.Variants, func(s string) bool { return s != "" }),
		VariantCaseSensitive: utility.FromBoolTPtr(options.TaskCaseSensitive), // Default to true for performance reasons.
	}

	mostRecentWaterfallVersion, err := model.GetMostRecentWaterfallVersion(ctx, projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching most recent waterfall version: %s", err.Error()))
	}
	if mostRecentWaterfallVersion == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("no versions found for project '%s'", projectId))
	}

	var activeVersions []model.Version
	if len(opts.Tasks) > 0 || len(opts.Statuses) > 0 {
		var searchOffset int
		if opts.MaxOrder != 0 {
			searchOffset = opts.MaxOrder
		} else if opts.MinOrder != 0 {
			searchOffset = opts.MinOrder
		} else {
			// Add one because minOrder and maxOrder are exclusive
			searchOffset = mostRecentWaterfallVersion.RevisionOrderNumber + 1
		}
		activeVersions, err = model.GetActiveVersionsByTaskFilters(ctx, projectId, opts, searchOffset)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting active waterfall versions: %s", err.Error()))
		}
	} else {
		activeVersions, err = model.GetActiveWaterfallVersions(ctx, projectId, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting active waterfall versions: %s", err.Error()))
		}
	}

	// Since GetAllWaterfallVersions uses an inclusive order range ($gte instead of $gt), add 1 to our minimum range
	minVersionOrder := minOrderOpt + 1
	if len(activeVersions) == 0 {
		minVersionOrder = opts.MinOrder
	} else if minOrderOpt == 0 {
		// Find an older version that is activated. If it doesn't exist, that means there are trailing inactive
		// versions on the waterfall and that we should not place a lower bound.
		// If it does exist, set the min order to one more than its order. This is guaranteed to either be
		// the last version in activeVersions or the last inactive version within its collapsed group.
		prevActiveVersion, err := model.GetOlderActiveWaterfallVersion(ctx, projectId, activeVersions[len(activeVersions)-1])
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching previous active waterfall version: %s", err.Error()))
		}
		if prevActiveVersion == nil {
			minVersionOrder = 0
		} else {
			minVersionOrder = prevActiveVersion.RevisionOrderNumber + 1
		}
	}

	// Same as above, but subtract for max order
	maxVersionOrder := maxOrderOpt - 1
	if len(activeVersions) == 0 {
		maxVersionOrder = opts.MaxOrder
	} else if maxOrderOpt == 0 && minOrderOpt == 0 {
		// If no order options were specified, we're on the first page and should not put a limit on the first version returned so that we don't omit inactive versions
		maxVersionOrder = 0
	} else if maxOrderOpt == 0 {
		// Find a newer version that is activated. If it doesn't exist, that means there are leading inactive
		// versions on the waterfall and we should reset to the first page.
		// If it does exist, set the max order to one less than its order. This is guaranteed to either be
		// the first version in activeVersions or the first inactive version within its collapsed group.
		nextActiveVersion, err := model.GetNewerActiveWaterfallVersion(ctx, projectId, activeVersions[0])
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching next active waterfall version: %s", err.Error()))
		}
		if nextActiveVersion == nil {
			maxVersionOrder = 0
		} else {
			maxVersionOrder = nextActiveVersion.RevisionOrderNumber - 1
		}
	}

	allVersions, err := model.GetAllWaterfallVersions(ctx, projectId, minVersionOrder, maxVersionOrder)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting waterfall versions: %s", err.Error()))
	}

	activeVersionIds := []string{}
	for _, v := range activeVersions {
		activeVersionIds = append(activeVersionIds, v.Id)
	}

	prevPageOrder := 0
	nextPageOrder := 0
	if len(allVersions) > 0 {
		// Return the min and max orders returned to be used as parameters for navigating to the next page
		prevPageOrder = allVersions[0].RevisionOrderNumber
		nextPageOrder = allVersions[len(allVersions)-1].RevisionOrderNumber

		// There's no previous page to navigate to if we've reached the most recent commit.
		if mostRecentWaterfallVersion.RevisionOrderNumber <= prevPageOrder {
			prevPageOrder = 0
		}
		// The first order of any project is 1, so there's no next page if we've reached that.
		if nextPageOrder == 1 {
			nextPageOrder = 0
		}
	}

	flattenedVersions := []*restModel.APIVersion{}
	for _, v := range allVersions {
		apiVersion := &restModel.APIVersion{}
		apiVersion.BuildFromService(ctx, v)
		flattenedVersions = append(flattenedVersions, apiVersion)
	}

	results := &Waterfall{
		FlattenedVersions: flattenedVersions,
		Pagination: &WaterfallPagination{
			ActiveVersionIds:       activeVersionIds,
			NextPageOrder:          nextPageOrder,
			PrevPageOrder:          prevPageOrder,
			MostRecentVersionOrder: mostRecentWaterfallVersion.RevisionOrderNumber,
			HasNextPage:            nextPageOrder > 0,
			HasPrevPage:            prevPageOrder > 0,
		},
	}

	return results, nil
}

// TaskHistory is the resolver for the taskHistory field.
func (r *queryResolver) TaskHistory(ctx context.Context, options TaskHistoryOpts) (*TaskHistory, error) {
	// CursorParams orient the query around a specific task (e.g. fetch 50 tasks before task A). Without CursorParams,
	// we don't have enough information about what tasks to fetch.
	if options.CursorParams == nil {
		return nil, InputValidationError.Send(ctx, "must specify cursor params")
	}

	projectId, err := model.GetIdForProject(ctx, options.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", options.ProjectIdentifier, err.Error()))
	}

	taskID := options.CursorParams.CursorID
	includeCursor := options.CursorParams.IncludeCursor

	foundTask, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if foundTask == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	taskOrder := foundTask.RevisionOrderNumber

	opts := model.FindTaskHistoryOptions{
		TaskName:     options.TaskName,
		BuildVariant: options.BuildVariant,
		ProjectId:    projectId,
		Limit:        options.Limit,
	}

	if options.CursorParams.Direction == TaskHistoryDirectionBefore {
		if includeCursor {
			opts.UpperBound = utility.ToIntPtr(taskOrder)
		} else {
			opts.UpperBound = utility.ToIntPtr(taskOrder - 1)
		}
	}
	if options.CursorParams.Direction == TaskHistoryDirectionAfter {
		if includeCursor {
			opts.LowerBound = utility.ToIntPtr(taskOrder)
		} else {
			opts.LowerBound = utility.ToIntPtr(taskOrder + 1)
		}
	}

	if options.Date != nil {
		date := utility.FromTimePtr(options.Date)
		order, err := model.GetTaskOrderByDate(ctx, date, opts)
		if err != nil {
			graphql.AddError(ctx, PartialError.Send(ctx, err.Error()))
		} else {
			opts.UpperBound = utility.ToIntPtr(order)
			opts.LowerBound = nil
		}
	}

	tasks, err := model.FindTasksForHistory(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting history for task '%s' in project '%s' and build variant '%s': %s", options.TaskName, options.ProjectIdentifier, options.BuildVariant, err.Error()))
	}

	apiTasks := []*restModel.APITask{}
	for _, t := range tasks {
		apiTask := &restModel.APITask{}
		if err = apiTask.BuildFromService(ctx, &t, nil); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", t.Id, err.Error()))
		}
		apiTasks = append(apiTasks, apiTask)
	}

	latestTask, err := model.GetLatestMainlineTask(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching latest task for '%s' in project '%s' and build variant '%s': %s", options.TaskName, options.ProjectIdentifier, options.BuildVariant, err.Error()))
	}

	oldestTask, err := model.GetOldestMainlineTask(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching oldest task for '%s' in project '%s' and build variant '%s': %s", options.TaskName, options.ProjectIdentifier, options.BuildVariant, err.Error()))
	}

	return &TaskHistory{
		Tasks: apiTasks,
		Pagination: &TaskHistoryPagination{
			MostRecentTaskOrder: latestTask.RevisionOrderNumber,
			OldestTaskOrder:     oldestTask.RevisionOrderNumber,
		},
	}, nil
}

// HasVersion is the resolver for the hasVersion field.
func (r *queryResolver) HasVersion(ctx context.Context, patchID string) (bool, error) {
	v, err := model.VersionFindOne(ctx, model.VersionById(patchID).WithFields(model.VersionIdKey))
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", patchID, err.Error()))
	}
	if v != nil {
		return true, nil
	}

	if patch.IsValidId(patchID) {
		p, err := patch.FindOneId(ctx, patchID)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()))
		}
		if p != nil {
			return false, nil
		}
	}
	return false, ResourceNotFound.Send(ctx, fmt.Sprintf("patch or version '%s' not found", patchID))
}

// Version is the resolver for the version field.
func (r *queryResolver) Version(ctx context.Context, versionID string) (*restModel.APIVersion, error) {
	v, err := model.VersionFindOneIdWithBuildVariants(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	apiVersion := restModel.APIVersion{}
	apiVersion.BuildFromService(ctx, *v)
	return &apiVersion, nil
}

// Image is the resolver for the image field returning information about an image including kernel, version, ami, name, and last deployed time.
func (r *queryResolver) Image(ctx context.Context, imageID string) (*restModel.APIImage, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	result, err := c.GetImageInfo(ctx, imageID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting image info for '%s': %s", imageID, err.Error()))
	}
	apiImage := restModel.APIImage{}
	apiImage.BuildFromService(*result)
	return &apiImage, nil
}

// Images is the resolver for the images field.
func (r *queryResolver) Images(ctx context.Context) ([]string, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	return c.GetImageNames(ctx)
}

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
