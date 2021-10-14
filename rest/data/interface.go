package data

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
)

// FindTestsByTaskIdOpts contains filtering, sorting and pagination options for TestResults.
type FindTestsByTaskIdOpts struct {
	Execution int
	GroupID   string
	Limit     int
	Page      int
	// SortBy should equal a bson tag from the TestResults struct.
	SortBy   string
	SortDir  int
	Statuses []string
	// TaskID is the only required field.
	TaskID string
	// TestID matches all IDs >= TestID.
	TestID   string
	TestName string
}

// Connector is an interface that contains all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type Connector interface {
	// Get and Set URL provide access to the main url string of the API.
	GetURL() string
	SetURL(string)

	// Get and Set Prefix provide access to the prefix that prepends all of the
	// URL paths.
	GetPrefix() string
	SetPrefix(string)

	// FindTaskById is a method to find a specific task given its ID.
	FindTaskById(string) (*task.Task, error)
	FindTaskByIdAndExecution(string, int) (*task.Task, error)
	FindTaskWithinTimePeriod(time.Time, time.Time, string, []string) ([]task.Task, error)
	FindOldTasksByIDWithDisplayTasks(string) ([]task.Task, error)
	FindTasksByIds([]string) ([]task.Task, error)
	SetTaskPriority(*task.Task, string, int64) error
	SetTaskActivated(string, string, bool) error
	ResetTask(string, string) error
	AbortTask(string, string) error
	CheckTaskSecret(string, *http.Request) (int, error)

	// FindTasksByBuildId is a method to find a set of tasks which all have the same
	// BuildId. It takes the buildId being queried for as its first parameter,
	// as well as a taskId, status, and limit for paginating through the results.
	FindTasksByBuildId(string, string, string, int, int) ([]task.Task, error)

	// FindBuildById is a method to find the build matching the same BuildId.
	FindBuildById(string) (*build.Build, error)
	// GetManifestByTask is a method to get the manifest for the given task.
	GetManifestByTask(string) (*manifest.Manifest, error)

	// SetBuildPriority and SetBuildActivated change the status of the input build
	SetBuildPriority(string, int64, string) error
	SetBuildActivated(string, string, bool) error

	// AbortBuild is a method to abort the build matching the same BuildId.
	AbortBuild(string, string) error
	// RestartBuild is a method to restart the build matching the same BuildId.
	RestartBuild(string, string) error

	// Find project variables matching given projectId (merged with variables matching the repoId, if given).
	// If bool is set, also return private variables.
	FindProjectVarsById(string, string, bool) (*restModel.APIProjectVars, error)
	// UpdateProjectVars updates the project using the variables given
	// in the model (removing old variables if overwrite is set).
	// If successful, updates the given projectVars with the updated projectVars.
	UpdateProjectVars(string, *restModel.APIProjectVars, bool) error
	UpdateProjectVarsByValue(string, string, string, bool) (map[string]string, error)
	// CopyProjectVars copies the variables for the first project to the second
	CopyProjectVars(string, string) error

	// Find the project matching the given ProjectId. Includes repo settings if set.
	FindProjectById(string, bool) (*model.ProjectRef, error)
	// Create/Update a project the given projectRef
	CreateProject(*model.ProjectRef, *user.DBUser) error
	UpdateProject(*model.ProjectRef) error
	// CopyProject copies the passed in project with the given project identifier, and returns the new project.
	CopyProject(context.Context, *model.ProjectRef, string) (*restModel.APIProjectRef, error)
	GetProjectAliasResults(*model.Project, string, bool) ([]restModel.APIVariantTasks, error)

	UpdateRepo(*model.RepoRef) error

	// GetProjectFromFile finds the file for the projectRef and returns the translated project, using the given token
	GetProjectFromFile(context.Context, model.ProjectRef, string, string) (*model.Project, *model.ParserProject, error)

	// EnableWebhooks creates a webhook for the project's owner/repo if one does not exist.
	// If unable to setup the new webhook, returns false but no error.
	EnableWebhooks(context.Context, *model.ProjectRef) (bool, error)
	// EnablePRTesting determines if PR testing can be enabled for the given project.
	EnablePRTesting(*model.ProjectRef) error

	// UpdateProjectRevision updates the given project's revision
	UpdateProjectRevision(string, string) error
	// FindProjects is a method to find projects as ordered by name
	FindProjects(string, int, int) ([]model.ProjectRef, error)
	GetProjectWithCommitQueueByOwnerRepoAndBranch(string, string, string) (*model.ProjectRef, error)
	FindEnabledProjectRefsByOwnerAndRepo(string, string) ([]model.ProjectRef, error)
	RemoveAdminFromProjects(string) error

	// GetVersionsAndVariants returns recent versions for a project
	GetVersionsAndVariants(int, int, *model.Project) (*restModel.VersionVariantData, error)
	// GetProjectVersionsWithOptions returns the batch of project versions
	GetProjectVersionsWithOptions(string, model.GetVersionsOptions) ([]restModel.APIVersion, error)
	GetProjectEventLog(string, time.Time, int) ([]restModel.APIProjectEvent, error)
	CreateVersionFromConfig(context.Context, *model.ProjectInfo, model.VersionMetadata, bool) (*model.Version, error)

	// Given a version and a project ID, return the translated project and the intermediate project.
	LoadProjectForVersion(*model.Version, string) (*model.Project, *model.ParserProject, error)
	// FindByProjectAndCommit is a method to find a set of tasks which ran as part of
	// certain version in a project. It takes the projectId, commit hash, and a taskId
	// for paginating through the results.
	FindTasksByProjectAndCommit(string, string, string, string, int) ([]task.Task, error)

	// FindTestById returns a single test result with the given id.
	FindTestById(string) ([]testresult.TestResult, error)
	// FindTestsByTaskId returns a paginated list of TestResults from a required TaskID.
	FindTestsByTaskId(FindTestsByTaskIdOpts) ([]testresult.TestResult, error)
	GetTestCountByTaskIdAndFilters(string, string, []string, int) (int, error)
	FindTasksByVersion(string, TaskFilterOptions) ([]task.Task, int, error)
	// FindUserById is a method to find a specific user given its ID.
	FindUserById(string) (gimlet.User, error)
	//FindUserByGithubName is a method to find a user given their Github name, if configured.
	FindUserByGithubName(string) (gimlet.User, error)
	GetServiceUsers() ([]restModel.APIDBUser, error)
	AddOrUpdateServiceUser(restModel.APIDBUser) error
	DeleteServiceUser(id string) error
	// FindHostsById is a method to find a sorted list of hosts given an ID to
	// start from.
	FindHostsById(string, string, string, int) ([]host.Host, error)

	// FindHostsInRange is a method to find a filtered list of hosts
	FindHostsInRange(restModel.APIHostParams, string) ([]host.Host, error)

	FindHostById(string) (*host.Host, error)

	FindHostByIpAddress(string) (*host.Host, error)

	// FindHostByIdWithOwner finds a host with given host ID that was
	// started by the given user. If the given user is a super-user,
	// the host will also be returned regardless of who the host was
	// started by
	FindHostByIdWithOwner(string, gimlet.User) (*host.Host, error)

	// FindHostsByDistro finds all hosts that have either a matching distro ID
	// or distro alias.
	FindHostsByDistro(string) ([]host.Host, error)

	// FindHostWithVolume returns the host that has the given volume attached
	FindHostWithVolume(string) (*host.Host, error)

	// GetRunningHosts gets paginated running hosts and applies any filters
	GetPaginatedRunningHosts(hostID, distroID, currentTaskID string, statuses []string, startedBy string, sortBy string, sortDir, page, limit int) ([]host.Host, *int, int, error)

	// Find a host by ID or Tag and queries for full running task
	GetHostByIdOrTagWithTask(hostID string) (*host.Host, error)

	// GenerateHostProvisioningScript generates and returns the script to
	// provision the host given by host ID.
	GenerateHostProvisioningScript(ctx context.Context, hostID string) (string, error)

	// NewIntentHost is a method to insert an intent host given a distro and the name of a saved public key
	NewIntentHost(context.Context, *restModel.HostRequestOptions, *user.DBUser, *evergreen.Settings) (*host.Host, error)

	FindVolumeById(string) (*host.Volume, error)
	FindVolumesByUser(string) ([]host.Volume, error)
	SetVolumeName(*host.Volume, string) error

	// AggregateSpawnhostData returns basic metrics on spawn host/volume usage.
	AggregateSpawnhostData() (*host.SpawnHostUsage, error)
	// FetchContext is a method to fetch a context given a series of identifiers.
	FetchContext(string, string, string, string, string) (model.Context, error)

	// FindDistroById is a method to find the distro matching the given distroId.
	FindDistroById(string) (*distro.Distro, error)
	// FindAllDistros is a method to find a sorted list of all distros.
	FindAllDistros() ([]distro.Distro, error)
	// UpdateDistro is a method that updates a given distro
	UpdateDistro(*distro.Distro, *distro.Distro) error
	// FindDistroById is a method to delete the distro matching the given distroId.
	DeleteDistroById(string) error
	// CreateDistro is a method to insert a given distro.
	CreateDistro(distro *distro.Distro) error

	// ClearTaskQueue deletes all tasks from the task queue for a distro
	ClearTaskQueue(string) error

	// FindVersionById returns version given its ID.
	FindVersionById(string) (*model.Version, error)

	// FindVersionByProjectAndRevision returns the version with this project and revision
	FindVersionByProjectAndRevision(string, string) (*model.Version, error)
	// AddGitTagToVersion adds a git tag to a version document
	AddGitTagToVersion(string, model.GitTag) error

	// FindPatchesByProject provides access to the patches corresponding to the input project ID
	// as ordered by creation time.
	FindPatchesByProject(string, time.Time, int) ([]restModel.APIPatch, error)
	// FindPatchByUser finds patches for the input user as ordered by creation time
	FindPatchesByUser(string, time.Time, int) ([]restModel.APIPatch, error)
	// FindPatchesByUserPatchNameStatusesCommitQueue fetches a page of patches corresponding to the input user ID
	// as ordered by creation time and filtered by given statuses, patch name commit queue parameter
	FindPatchesByUserPatchNameStatusesCommitQueue(string, string, []string, bool, int, int) ([]restModel.APIPatch, *int, error)
	// FindPatchesByProjectPatchNameStatusesCommitQueue fetches a page of patches corresponding to the input project ID
	// as ordered by creation time and filtered by given statuses, patch name commit queue parameter
	FindPatchesByProjectPatchNameStatusesCommitQueue(string, string, []string, bool, int, int) ([]restModel.APIPatch, *int, error)
	// FindPatchById fetches the patch corresponding to the input patch ID.
	FindPatchById(string) (*restModel.APIPatch, error)
	// FindPatchById fetches the patch corresponding to the input patch ID.
	GetChildPatchIds(string) ([]string, error)
	//FindPatchesByIds fetches an array of patches that corresponding to the input patch IDs
	FindPatchesByIds([]string) ([]restModel.APIPatch, error)
	// GetPatchRawPatches fetches the raw patches for a patch
	GetPatchRawPatches(string) (map[string]string, error)
	// AbortVersion aborts all tasks of a version given its ID.
	AbortVersion(string, string) error

	// AbortPatch aborts the patch corresponding to the input patch ID and deletes if not finalized.
	AbortPatch(string, string) error
	// AbortPatchesFromPullRequest aborts patches with the same PR Number,
	// in the same repository, at the pull request's close time
	AbortPatchesFromPullRequest(*github.PullRequestEvent) error

	// RestartVersion restarts all completed tasks of a version given its ID and the caller.
	RestartVersion(string, string) error
	// SetPatchPriority and SetPatchActivated change the status of the input patch
	SetPatchPriority(string, int64, string) error
	SetPatchActivated(context.Context, string, string, bool, *evergreen.Settings) error

	// GetEvergreenSettings/SetEvergreenSettings retrieves/sets the system-wide settings document
	GetEvergreenSettings() (*evergreen.Settings, error)
	GetBanner() (string, string, error)
	SetEvergreenSettings(*restModel.APIAdminSettings, *evergreen.Settings, *user.DBUser, bool) (*evergreen.Settings, error)
	// SetAdminBanner sets set the banner in the system-wide settings document
	SetAdminBanner(string, *user.DBUser) error
	// SetBannerTheme sets set the banner theme in the system-wide settings document
	SetBannerTheme(string, *user.DBUser) error
	// SetAdminBanner sets set the service flags in the system-wide settings document
	SetServiceFlags(evergreen.ServiceFlags, *user.DBUser) error
	RestartFailedTasks(amboy.Queue, model.RestartOptions) (*restModel.RestartResponse, error)
	//RestartFailedCommitQueueVersions takes in a time range
	RestartFailedCommitQueueVersions(opts model.RestartOptions) (*restModel.RestartResponse, error)
	RevertConfigTo(string, string) error
	GetAdminEventLog(time.Time, int) ([]restModel.APIAdminEvent, error)
	MapLDAPGroupToRole(string, string) error
	UnmapLDAPGroupToRole(string) error

	// FindRecentTasks finds tasks that have recently finished.
	FindRecentTasks(int) ([]task.Task, *task.ResultCounts, error)
	FindRecentTaskListDistro(int) (*restModel.APIRecentTaskStatsList, error)
	FindRecentTaskListProject(int) (*restModel.APIRecentTaskStatsList, error)
	FindRecentTaskListAgentVersion(int) (*restModel.APIRecentTaskStatsList, error)
	// GetHostStatsByDistro returns host stats broken down by distro
	GetHostStatsByDistro() ([]host.StatsByDistro, error)

	AddPublicKey(*user.DBUser, string, string) error
	DeletePublicKey(*user.DBUser, string) error
	GetPublicKey(*user.DBUser, string) (string, error)
	UpdateSettings(*user.DBUser, user.UserSettings) error
	SubmitFeedback(restModel.APIFeedbackSubmission) error

	AddPatchIntent(patch.Intent, amboy.Queue) error

	SetHostStatus(*host.Host, string, string) error
	SetHostExpirationTime(*host.Host, time.Time) error

	// TerminateHost terminates the given host via the cloud provider's API
	TerminateHost(context.Context, *host.Host, string) error

	// DisableHost disables the host, notifies it's been disabled,
	// and clears and resets its running task.
	DisableHost(context.Context, evergreen.Environment, *host.Host, string) error

	CheckHostSecret(string, *http.Request) (int, error)

	// CreatePod creates a new pod and returns the result of creating the pod.
	CreatePod(restModel.APICreatePod) (*restModel.APICreatePodResponse, error)
	// FindPodByID finds a pod by the given ID.
	FindPodByID(id string) (*restModel.APIPod, error)
	// FindPodByExternalID finds a pod by the given external identifier.
	FindPodByExternalID(id string) (*restModel.APIPod, error)
	// UpdatePodStatus updates a pod's status by ID.
	UpdatePodStatus(id string, current, updated restModel.APIPodStatus) error
	// CheckPodSecret checks that the ID and secret match the server's
	// stored credentials for the pod.
	CheckPodSecret(id, secret string) error

	// FindProjectAliases queries the database to find all aliases (including or excluding those specified).
	// Includes repo aliases if repoId is provided.
	FindProjectAliases(string, string, []restModel.APIProjectAlias) ([]restModel.APIProjectAlias, error)
	// CopyProjectAliases copies aliases from the first project for the second project.
	CopyProjectAliases(string, string) error
	// UpdateProjectAliases upserts/deletes aliases for the given project
	UpdateProjectAliases(string, []restModel.APIProjectAlias) error
	// UpdateAliasesForSection, given a project, a list of current aliases, a list of previous aliases, and a project page section,
	// upserts any current aliases, and deletes any aliases that existed previously but not anymore (only
	// considers the aliases that are relevant for the section). Returns if any aliases have been modified.
	UpdateAliasesForSection(string, []restModel.APIProjectAlias, []model.ProjectAlias, model.ProjectPageSection) (bool, error)
	// HasMatchingGitTagAliasAndRemotePath returns true if the project has aliases defined that match the given tag, and
	// returns the remote path if applicable
	HasMatchingGitTagAliasAndRemotePath(string, string) (bool, string, error)
	// TriggerRepotracker creates an amboy job to get the commits from a
	// Github Push Event
	TriggerRepotracker(amboy.Queue, string, *github.PushEvent) error

	// GetCLIUpdate fetches the current cli version and the urls to download
	GetCLIUpdate() (*restModel.APICLIUpdate, error)

	// GenerateTasks parses JSON files for `generate.tasks` and creates the new builds and tasks.
	GenerateTasks(context.Context, string, []json.RawMessage, amboy.QueueGroup) error

	// GeneratePoll checks to see if a `generate.tasks` job has finished.
	GeneratePoll(context.Context, string, amboy.QueueGroup) (bool, []string, error)

	// SaveSubscriptions, given an owner, a list of current subscriptions and an isProject boolean and
	// upserts the given set of API subscriptions (if the owner is a project type, verify that all current subscriptions have
	// the right owner and owner type).
	SaveSubscriptions(string, []restModel.APISubscription, bool) error
	// GetSubscriptions returns the subscriptions that belong to a user
	GetSubscriptions(string, event.OwnerType) ([]restModel.APISubscription, error)
	DeleteSubscriptions(string, []string) error
	// CopyProjectSubscriptions copies subscriptions from the first project for the second project.
	CopyProjectSubscriptions(string, string) error
	UpdateAdminRoles(*model.ProjectRef, []string, []string) error

	// Notifications
	GetNotificationsStats() (*restModel.APIEventStats, error)

	// ListHostsForTask lists running hosts scoped to the task or the task's build.
	ListHostsForTask(context.Context, string) ([]host.Host, error)
	MakeIntentHost(string, string, string, apimodels.CreateHost) (*host.Host, error)
	CreateHostsFromTask(*task.Task, user.DBUser, string) error

	// Get test execution statistics
	GetTestStats(stats.StatsFilter) ([]restModel.APITestStats, error)
	GetTaskStats(stats.StatsFilter) ([]restModel.APITaskStats, error)

	// Get task reliability scores
	GetTaskReliabilityScores(reliability.TaskReliabilityFilter) ([]restModel.APITaskReliability, error)

	// Commit queue methods
	// GetGithubPR takes the owner, repo, and PR number.
	GetGitHubPR(context.Context, string, string, int) (*github.PullRequest, error)
	// if bool is true, move the commit queue item to be processed next.
	EnqueueItem(string, restModel.APICommitQueueItem, bool) (int, error)
	AddPatchForPr(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (string, error)
	FindCommitQueueForProject(string) (*restModel.APICommitQueue, error)
	EnableCommitQueue(*model.ProjectRef) error
	CommitQueueRemoveItem(string, string, string) (*restModel.APICommitQueueItem, error)
	IsItemOnCommitQueue(string, string) (bool, error)
	CommitQueueClearAll() (int, error)
	CreatePatchForMerge(context.Context, string, string) (*restModel.APIPatch, error)
	IsPatchEmpty(string) (bool, error)
	IsAuthorizedToPatchAndMerge(context.Context, *evergreen.Settings, UserRepoInfo) (bool, error)
	GetMessageForPatch(string) (string, error)
	ConcludeMerge(string, string) error
	GetAdditionalPatches(patchId string) ([]string, error)

	// GetDockerLogs returns logs for the given docker container
	GetDockerLogs(context.Context, string, *host.Host, *evergreen.Settings, types.ContainerLogsOptions) (io.Reader, error)
	// GetDockerStatus returns the status of the given docker container
	GetDockerStatus(context.Context, string, *host.Host, *evergreen.Settings) (*cloud.ContainerStatus, error)

	//GetProjectSettings returns the ProjectSettings of the given identifier and ProjectRef
	GetProjectSettings(p *model.ProjectRef) (*model.ProjectSettings, error)
	// SaveProjectSettingsForSection saves the given UI page section and logs it for the given user. If isRepo is true, uses
	// RepoRef related functions and collection instead of ProjectRef.
	SaveProjectSettingsForSection(context.Context, string, *restModel.APIProjectSettings, model.ProjectPageSection, bool, string) (*restModel.APIProjectSettings, error)

	// CompareTasks returns the order that the given tasks would be scheduled, along with the scheduling logic.
	CompareTasks([]string, bool) ([]string, map[string]map[string]string, error)
}
