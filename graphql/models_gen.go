// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package graphql

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
)

type AbortInfo struct {
	BuildVariantDisplayName string `json:"buildVariantDisplayName"`
	NewVersion              string `json:"newVersion"`
	PrClosed                bool   `json:"prClosed"`
	TaskDisplayName         string `json:"taskDisplayName"`
	TaskID                  string `json:"taskID"`
	User                    string `json:"user"`
}

// Build Baron is a service that can be integrated into a project (see Confluence Wiki for more details).
// This type is returned from the buildBaron query, and contains information about Build Baron configurations and suggested
// tickets from JIRA for a given task on a given execution.
type BuildBaron struct {
	BbTicketCreationDefined bool                         `json:"bbTicketCreationDefined"`
	BuildBaronConfigured    bool                         `json:"buildBaronConfigured"`
	SearchReturnInfo        *thirdparty.SearchReturnInfo `json:"searchReturnInfo"`
}

// BuildVariantOptions is an input to the mainlineCommits query.
// It stores values for statuses, tasks, and variants which are used to filter for matching versions.
type BuildVariantOptions struct {
	IncludeBaseTasks *bool    `json:"includeBaseTasks"`
	Statuses         []string `json:"statuses"`
	Tasks            []string `json:"tasks"`
	Variants         []string `json:"variants"`
}

type Dependency struct {
	BuildVariant   string         `json:"buildVariant"`
	MetStatus      MetStatus      `json:"metStatus"`
	Name           string         `json:"name"`
	RequiredStatus RequiredStatus `json:"requiredStatus"`
	TaskID         string         `json:"taskId"`
}

type DisplayTask struct {
	ExecTasks []string `json:"ExecTasks"`
	Name      string   `json:"Name"`
}

// EditSpawnHostInput is the input to the editSpawnHost mutation.
// Its fields determine how a given host will be modified.
type EditSpawnHostInput struct {
	AddedInstanceTags   []*host.Tag     `json:"addedInstanceTags"`
	DeletedInstanceTags []*host.Tag     `json:"deletedInstanceTags"`
	DisplayName         *string         `json:"displayName"`
	Expiration          *time.Time      `json:"expiration"`
	HostID              string          `json:"hostId"`
	InstanceType        *string         `json:"instanceType"`
	NoExpiration        *bool           `json:"noExpiration"`
	PublicKey           *PublicKeyInput `json:"publicKey"`
	SavePublicKey       *bool           `json:"savePublicKey"`
	ServicePassword     *string         `json:"servicePassword"`
	Volume              *string         `json:"volume"`
}

type ExternalPatchLink struct {
	URL         string `json:"url"`
	DisplayName string `json:"displayName"`
}

type GroupedBuildVariant struct {
	DisplayName string           `json:"displayName"`
	Tasks       []*model.APITask `json:"tasks"`
	Variant     string           `json:"variant"`
}

type GroupedFiles struct {
	Files    []*model.APIFile `json:"files"`
	TaskName *string          `json:"taskName"`
}

// GroupedProjects is the return value for the projects & viewableProjectRefs queries.
// It contains an array of projects which are grouped under a groupDisplayName.
type GroupedProjects struct {
	GroupDisplayName string                 `json:"groupDisplayName"`
	Projects         []*model.APIProjectRef `json:"projects"`
	Repo             *model.APIProjectRef   `json:"repo"`
}

// HostEvents is the return value for the hostEvents query.
// It contains the event log entries for a given host.
type HostEvents struct {
	Count           int                           `json:"count"`
	EventLogEntries []*model.HostAPIEventLogEntry `json:"eventLogEntries"`
}

// HostsResponse is the return value for the hosts query.
// It contains an array of Hosts matching the filter conditions, as well as some count information.
type HostsResponse struct {
	FilteredHostsCount *int             `json:"filteredHostsCount"`
	Hosts              []*model.APIHost `json:"hosts"`
	TotalHostsCount    int              `json:"totalHostsCount"`
}

type MainlineCommitVersion struct {
	RolledUpVersions []*model.APIVersion `json:"rolledUpVersions"`
	Version          *model.APIVersion   `json:"version"`
}

// MainlineCommits is returned by the mainline commits query.
// It contains information about versions (both unactivated and activated) which is surfaced on the Project Health page.
type MainlineCommits struct {
	NextPageOrderNumber *int                     `json:"nextPageOrderNumber"`
	PrevPageOrderNumber *int                     `json:"prevPageOrderNumber"`
	Versions            []*MainlineCommitVersion `json:"versions"`
}

// MainlineCommitsOptions is an input to the mainlineCommits query.
// Its fields determine what mainline commits we fetch for a given projectID.
type MainlineCommitsOptions struct {
	Limit             *int     `json:"limit"`
	ProjectIdentifier string   `json:"projectIdentifier"`
	Requesters        []string `json:"requesters"`
	ShouldCollapse    *bool    `json:"shouldCollapse"`
	SkipOrderNumber   *int     `json:"skipOrderNumber"`
}

type Manifest struct {
	ID              string                 `json:"id"`
	Branch          string                 `json:"branch"`
	IsBase          bool                   `json:"isBase"`
	ModuleOverrides map[string]string      `json:"moduleOverrides"`
	Modules         map[string]interface{} `json:"modules"`
	Project         string                 `json:"project"`
	Revision        string                 `json:"revision"`
}

// MoveProjectInput is the input to the attachProjectToNewRepo mutation.
// It contains information used to move a project to a a new owner and repo.
type MoveProjectInput struct {
	NewOwner  string `json:"newOwner"`
	NewRepo   string `json:"newRepo"`
	ProjectID string `json:"projectId"`
}

// PatchConfigure is the input to the schedulePatch mutation.
// It contains information about how a user has configured their patch (e.g. name, tasks to run, etc).
type PatchConfigure struct {
	Description         string                `json:"description"`
	Parameters          []*model.APIParameter `json:"parameters"`
	PatchTriggerAliases []string              `json:"patchTriggerAliases"`
	VariantsTasks       []*VariantTasks       `json:"variantsTasks"`
}

type PatchDuration struct {
	Makespan  *string    `json:"makespan"`
	Time      *PatchTime `json:"time"`
	TimeTaken *string    `json:"timeTaken"`
}

type PatchProject struct {
	Variants []*ProjectBuildVariant `json:"variants"`
}

type PatchTime struct {
	Finished    *string `json:"finished"`
	Started     *string `json:"started"`
	SubmittedAt string  `json:"submittedAt"`
}

// Patches is the return value of the patches field for the User and Project types.
// It contains an array Patches for either an individual user or a project.
type Patches struct {
	FilteredPatchCount int               `json:"filteredPatchCount"`
	Patches            []*model.APIPatch `json:"patches"`
}

// PatchesInput is the input value to the patches field for the User and Project types.
// Based on the information in PatchesInput, we return a list of Patches for either an individual user or a project.
type PatchesInput struct {
	IncludeCommitQueue *bool    `json:"includeCommitQueue"`
	Limit              int      `json:"limit"`
	OnlyCommitQueue    *bool    `json:"onlyCommitQueue"`
	Page               int      `json:"page"`
	PatchName          string   `json:"patchName"`
	Statuses           []string `json:"statuses"`
}

type Permissions struct {
	CanCreateProject bool   `json:"canCreateProject"`
	UserID           string `json:"userId"`
}

type ProjectBuildVariant struct {
	DisplayName string   `json:"displayName"`
	Name        string   `json:"name"`
	Tasks       []string `json:"tasks"`
}

// ProjectEvents contains project event log entries that concern the history of changes related to project
// settings.
// Although RepoSettings uses RepoRef in practice to have stronger types, this can't be enforced
// or event logs because new fields could always be introduced that don't exist in the old event logs.
type ProjectEvents struct {
	Count           int                      `json:"count"`
	EventLogEntries []*model.APIProjectEvent `json:"eventLogEntries"`
}

// PublicKeyInput is an input to the createPublicKey and updatePublicKey mutations.
type PublicKeyInput struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// SortOrder[] is an input value for version.tasks. It is used to define whether to sort by ASC/DEC for a given sort key.
type SortOrder struct {
	Direction SortDirection    `json:"Direction"`
	Key       TaskSortCategory `json:"Key"`
}

// SpawnHostInput is the input to the spawnHost mutation.
// Its fields determine the properties of the host that will be spawned.
type SpawnHostInput struct {
	DistroID                string          `json:"distroId"`
	Expiration              *time.Time      `json:"expiration"`
	HomeVolumeSize          *int            `json:"homeVolumeSize"`
	IsVirtualWorkStation    bool            `json:"isVirtualWorkStation"`
	NoExpiration            bool            `json:"noExpiration"`
	PublicKey               *PublicKeyInput `json:"publicKey"`
	Region                  string          `json:"region"`
	SavePublicKey           bool            `json:"savePublicKey"`
	SetUpScript             *string         `json:"setUpScript"`
	SpawnHostsStartedByTask *bool           `json:"spawnHostsStartedByTask"`
	TaskID                  *string         `json:"taskId"`
	TaskSync                *bool           `json:"taskSync"`
	UseProjectSetupScript   *bool           `json:"useProjectSetupScript"`
	UserDataScript          *string         `json:"userDataScript"`
	UseTaskConfig           *bool           `json:"useTaskConfig"`
	VolumeID                *string         `json:"volumeId"`
}

// SpawnVolumeInput is the input to the spawnVolume mutation.
// Its fields determine the properties of the volume that will be spawned.
type SpawnVolumeInput struct {
	AvailabilityZone string     `json:"availabilityZone"`
	Expiration       *time.Time `json:"expiration"`
	Host             *string    `json:"host"`
	NoExpiration     *bool      `json:"noExpiration"`
	Size             int        `json:"size"`
	Type             string     `json:"type"`
}

type Subscriber struct {
	EmailSubscriber       *string                         `json:"emailSubscriber"`
	GithubCheckSubscriber *model.APIGithubCheckSubscriber `json:"githubCheckSubscriber"`
	GithubPRSubscriber    *model.APIGithubPRSubscriber    `json:"githubPRSubscriber"`
	JiraCommentSubscriber *string                         `json:"jiraCommentSubscriber"`
	JiraIssueSubscriber   *model.APIJIRAIssueSubscriber   `json:"jiraIssueSubscriber"`
	SlackSubscriber       *string                         `json:"slackSubscriber"`
	WebhookSubscriber     *model.APIWebhookSubscriber     `json:"webhookSubscriber"`
}

// TaskFiles is the return value for the taskFiles query.
// Some tasks generate files which are represented by this type.
type TaskFiles struct {
	FileCount    int             `json:"fileCount"`
	GroupedFiles []*GroupedFiles `json:"groupedFiles"`
}

// TaskFilterOptions defines the parameters that are used when fetching tasks from a Version.
type TaskFilterOptions struct {
	BaseStatuses               []string     `json:"baseStatuses"`
	IncludeEmptyActivation     *bool        `json:"includeEmptyActivation"`
	IncludeNeverActivatedTasks *bool        `json:"includeNeverActivatedTasks"`
	Limit                      *int         `json:"limit"`
	Page                       *int         `json:"page"`
	Sorts                      []*SortOrder `json:"sorts"`
	Statuses                   []string     `json:"statuses"`
	TaskName                   *string      `json:"taskName"`
	Variant                    *string      `json:"variant"`
}

// TaskLogs is the return value for the task.taskLogs query.
// It contains the logs for a given task on a given execution.
type TaskLogs struct {
	AgentLogs     []*apimodels.LogMessage       `json:"agentLogs"`
	AllLogs       []*apimodels.LogMessage       `json:"allLogs"`
	DefaultLogger string                        `json:"defaultLogger"`
	EventLogs     []*model.TaskAPIEventLogEntry `json:"eventLogs"`
	Execution     int                           `json:"execution"`
	SystemLogs    []*apimodels.LogMessage       `json:"systemLogs"`
	TaskID        string                        `json:"taskId"`
	TaskLogs      []*apimodels.LogMessage       `json:"taskLogs"`
}

// TaskQueueDistro[] is the return value for the taskQueueDistros query.
// It contains information about how many tasks and hosts are running on on a particular distro.
type TaskQueueDistro struct {
	ID        string `json:"id"`
	HostCount int    `json:"hostCount"`
	TaskCount int    `json:"taskCount"`
}

// TaskTestResult is the return value for the taskTests query.
// It contains the test results for a task. For example, if there is a task to run all unit tests, then the test results
// could be the result of each individual unit test.
type TaskTestResult struct {
	FilteredTestCount int              `json:"filteredTestCount"`
	TestResults       []*model.APITest `json:"testResults"`
	TotalTestCount    int              `json:"totalTestCount"`
}

// TaskTestResultSample is the return value for the taskTestSample query.
// It is used to represent failing test results on the task history pages.
type TaskTestResultSample struct {
	Execution               int      `json:"execution"`
	MatchingFailedTestNames []string `json:"matchingFailedTestNames"`
	TaskID                  string   `json:"taskId"`
	TotalTestCount          int      `json:"totalTestCount"`
}

// TestFilter is an input value for the taskTestSample query.
// It's used to filter for tests with testName and status testStatus.
type TestFilter struct {
	TestName   string `json:"testName"`
	TestStatus string `json:"testStatus"`
}

// UpdateVolumeInput is the input to the updateVolume mutation.
// Its fields determine how a given volume will be modified.
type UpdateVolumeInput struct {
	Expiration   *time.Time `json:"expiration"`
	Name         *string    `json:"name"`
	NoExpiration *bool      `json:"noExpiration"`
	VolumeID     string     `json:"volumeId"`
}

type UpstreamProject struct {
	Owner       string            `json:"owner"`
	Project     string            `json:"project"`
	Repo        string            `json:"repo"`
	ResourceID  string            `json:"resourceID"`
	Revision    string            `json:"revision"`
	Task        *model.APITask    `json:"task"`
	TriggerID   string            `json:"triggerID"`
	TriggerType string            `json:"triggerType"`
	Version     *model.APIVersion `json:"version"`
}

// UserConfig is returned by the userConfig query.
// It contains configuration information such as the user's api key for the Evergreen CLI and a user's
// preferred UI (legacy vs Spruce).
type UserConfig struct {
	APIKey        string `json:"api_key"`
	APIServerHost string `json:"api_server_host"`
	UIServerHost  string `json:"ui_server_host"`
	User          string `json:"user"`
}

type VariantTasks struct {
	DisplayTasks []*DisplayTask `json:"displayTasks"`
	Tasks        []string       `json:"tasks"`
	Variant      string         `json:"variant"`
}

type VersionTasks struct {
	Count int              `json:"count"`
	Data  []*model.APITask `json:"data"`
}

type VersionTiming struct {
	Makespan  *model.APIDuration `json:"makespan"`
	TimeTaken *model.APIDuration `json:"timeTaken"`
}

// VolumeHost is the input to the attachVolumeToHost mutation.
// Its fields are used to attach the volume with volumeId to the host with hostId.
type VolumeHost struct {
	VolumeID string `json:"volumeId"`
	HostID   string `json:"hostId"`
}

type HostSortBy string

const (
	HostSortByID          HostSortBy = "ID"
	HostSortByCurrentTask HostSortBy = "CURRENT_TASK"
	HostSortByDistro      HostSortBy = "DISTRO"
	HostSortByElapsed     HostSortBy = "ELAPSED"
	HostSortByIDLeTime    HostSortBy = "IDLE_TIME"
	HostSortByOwner       HostSortBy = "OWNER"
	HostSortByStatus      HostSortBy = "STATUS"
	HostSortByUptime      HostSortBy = "UPTIME"
)

var AllHostSortBy = []HostSortBy{
	HostSortByID,
	HostSortByCurrentTask,
	HostSortByDistro,
	HostSortByElapsed,
	HostSortByIDLeTime,
	HostSortByOwner,
	HostSortByStatus,
	HostSortByUptime,
}

func (e HostSortBy) IsValid() bool {
	switch e {
	case HostSortByID, HostSortByCurrentTask, HostSortByDistro, HostSortByElapsed, HostSortByIDLeTime, HostSortByOwner, HostSortByStatus, HostSortByUptime:
		return true
	}
	return false
}

func (e HostSortBy) String() string {
	return string(e)
}

func (e *HostSortBy) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = HostSortBy(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid HostSortBy", str)
	}
	return nil
}

func (e HostSortBy) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type MetStatus string

const (
	MetStatusUnmet   MetStatus = "UNMET"
	MetStatusMet     MetStatus = "MET"
	MetStatusPending MetStatus = "PENDING"
	MetStatusStarted MetStatus = "STARTED"
)

var AllMetStatus = []MetStatus{
	MetStatusUnmet,
	MetStatusMet,
	MetStatusPending,
	MetStatusStarted,
}

func (e MetStatus) IsValid() bool {
	switch e {
	case MetStatusUnmet, MetStatusMet, MetStatusPending, MetStatusStarted:
		return true
	}
	return false
}

func (e MetStatus) String() string {
	return string(e)
}

func (e *MetStatus) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = MetStatus(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid MetStatus", str)
	}
	return nil
}

func (e MetStatus) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type ProjectSettingsAccess string

const (
	ProjectSettingsAccessEdit ProjectSettingsAccess = "EDIT"
	ProjectSettingsAccessView ProjectSettingsAccess = "VIEW"
)

var AllProjectSettingsAccess = []ProjectSettingsAccess{
	ProjectSettingsAccessEdit,
	ProjectSettingsAccessView,
}

func (e ProjectSettingsAccess) IsValid() bool {
	switch e {
	case ProjectSettingsAccessEdit, ProjectSettingsAccessView:
		return true
	}
	return false
}

func (e ProjectSettingsAccess) String() string {
	return string(e)
}

func (e *ProjectSettingsAccess) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = ProjectSettingsAccess(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid ProjectSettingsAccess", str)
	}
	return nil
}

func (e ProjectSettingsAccess) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type ProjectSettingsSection string

const (
	ProjectSettingsSectionGeneral              ProjectSettingsSection = "GENERAL"
	ProjectSettingsSectionAccess               ProjectSettingsSection = "ACCESS"
	ProjectSettingsSectionVariables            ProjectSettingsSection = "VARIABLES"
	ProjectSettingsSectionGithubAndCommitQueue ProjectSettingsSection = "GITHUB_AND_COMMIT_QUEUE"
	ProjectSettingsSectionNotifications        ProjectSettingsSection = "NOTIFICATIONS"
	ProjectSettingsSectionPatchAliases         ProjectSettingsSection = "PATCH_ALIASES"
	ProjectSettingsSectionWorkstation          ProjectSettingsSection = "WORKSTATION"
	ProjectSettingsSectionTriggers             ProjectSettingsSection = "TRIGGERS"
	ProjectSettingsSectionPeriodicBuilds       ProjectSettingsSection = "PERIODIC_BUILDS"
	ProjectSettingsSectionPlugins              ProjectSettingsSection = "PLUGINS"
	ProjectSettingsSectionContainers           ProjectSettingsSection = "CONTAINERS"
)

var AllProjectSettingsSection = []ProjectSettingsSection{
	ProjectSettingsSectionGeneral,
	ProjectSettingsSectionAccess,
	ProjectSettingsSectionVariables,
	ProjectSettingsSectionGithubAndCommitQueue,
	ProjectSettingsSectionNotifications,
	ProjectSettingsSectionPatchAliases,
	ProjectSettingsSectionWorkstation,
	ProjectSettingsSectionTriggers,
	ProjectSettingsSectionPeriodicBuilds,
	ProjectSettingsSectionPlugins,
	ProjectSettingsSectionContainers,
}

func (e ProjectSettingsSection) IsValid() bool {
	switch e {
	case ProjectSettingsSectionGeneral, ProjectSettingsSectionAccess, ProjectSettingsSectionVariables, ProjectSettingsSectionGithubAndCommitQueue, ProjectSettingsSectionNotifications, ProjectSettingsSectionPatchAliases, ProjectSettingsSectionWorkstation, ProjectSettingsSectionTriggers, ProjectSettingsSectionPeriodicBuilds, ProjectSettingsSectionPlugins, ProjectSettingsSectionContainers:
		return true
	}
	return false
}

func (e ProjectSettingsSection) String() string {
	return string(e)
}

func (e *ProjectSettingsSection) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = ProjectSettingsSection(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid ProjectSettingsSection", str)
	}
	return nil
}

func (e ProjectSettingsSection) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type RequiredStatus string

const (
	RequiredStatusMustFail    RequiredStatus = "MUST_FAIL"
	RequiredStatusMustFinish  RequiredStatus = "MUST_FINISH"
	RequiredStatusMustSucceed RequiredStatus = "MUST_SUCCEED"
)

var AllRequiredStatus = []RequiredStatus{
	RequiredStatusMustFail,
	RequiredStatusMustFinish,
	RequiredStatusMustSucceed,
}

func (e RequiredStatus) IsValid() bool {
	switch e {
	case RequiredStatusMustFail, RequiredStatusMustFinish, RequiredStatusMustSucceed:
		return true
	}
	return false
}

func (e RequiredStatus) String() string {
	return string(e)
}

func (e *RequiredStatus) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = RequiredStatus(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid RequiredStatus", str)
	}
	return nil
}

func (e RequiredStatus) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type SortDirection string

const (
	SortDirectionAsc  SortDirection = "ASC"
	SortDirectionDesc SortDirection = "DESC"
)

var AllSortDirection = []SortDirection{
	SortDirectionAsc,
	SortDirectionDesc,
}

func (e SortDirection) IsValid() bool {
	switch e {
	case SortDirectionAsc, SortDirectionDesc:
		return true
	}
	return false
}

func (e SortDirection) String() string {
	return string(e)
}

func (e *SortDirection) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = SortDirection(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid SortDirection", str)
	}
	return nil
}

func (e SortDirection) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type SpawnHostStatusActions string

const (
	SpawnHostStatusActionsStart     SpawnHostStatusActions = "START"
	SpawnHostStatusActionsStop      SpawnHostStatusActions = "STOP"
	SpawnHostStatusActionsTerminate SpawnHostStatusActions = "TERMINATE"
)

var AllSpawnHostStatusActions = []SpawnHostStatusActions{
	SpawnHostStatusActionsStart,
	SpawnHostStatusActionsStop,
	SpawnHostStatusActionsTerminate,
}

func (e SpawnHostStatusActions) IsValid() bool {
	switch e {
	case SpawnHostStatusActionsStart, SpawnHostStatusActionsStop, SpawnHostStatusActionsTerminate:
		return true
	}
	return false
}

func (e SpawnHostStatusActions) String() string {
	return string(e)
}

func (e *SpawnHostStatusActions) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = SpawnHostStatusActions(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid SpawnHostStatusActions", str)
	}
	return nil
}

func (e SpawnHostStatusActions) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type TaskQueueItemType string

const (
	TaskQueueItemTypeCommit TaskQueueItemType = "COMMIT"
	TaskQueueItemTypePatch  TaskQueueItemType = "PATCH"
)

var AllTaskQueueItemType = []TaskQueueItemType{
	TaskQueueItemTypeCommit,
	TaskQueueItemTypePatch,
}

func (e TaskQueueItemType) IsValid() bool {
	switch e {
	case TaskQueueItemTypeCommit, TaskQueueItemTypePatch:
		return true
	}
	return false
}

func (e TaskQueueItemType) String() string {
	return string(e)
}

func (e *TaskQueueItemType) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = TaskQueueItemType(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid TaskQueueItemType", str)
	}
	return nil
}

func (e TaskQueueItemType) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type TaskSortCategory string

const (
	TaskSortCategoryName       TaskSortCategory = "NAME"
	TaskSortCategoryStatus     TaskSortCategory = "STATUS"
	TaskSortCategoryBaseStatus TaskSortCategory = "BASE_STATUS"
	TaskSortCategoryVariant    TaskSortCategory = "VARIANT"
	TaskSortCategoryDuration   TaskSortCategory = "DURATION"
)

var AllTaskSortCategory = []TaskSortCategory{
	TaskSortCategoryName,
	TaskSortCategoryStatus,
	TaskSortCategoryBaseStatus,
	TaskSortCategoryVariant,
	TaskSortCategoryDuration,
}

func (e TaskSortCategory) IsValid() bool {
	switch e {
	case TaskSortCategoryName, TaskSortCategoryStatus, TaskSortCategoryBaseStatus, TaskSortCategoryVariant, TaskSortCategoryDuration:
		return true
	}
	return false
}

func (e TaskSortCategory) String() string {
	return string(e)
}

func (e *TaskSortCategory) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = TaskSortCategory(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid TaskSortCategory", str)
	}
	return nil
}

func (e TaskSortCategory) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type TestSortCategory string

const (
	TestSortCategoryBaseStatus TestSortCategory = "BASE_STATUS"
	TestSortCategoryStatus     TestSortCategory = "STATUS"
	TestSortCategoryStartTime  TestSortCategory = "START_TIME"
	TestSortCategoryDuration   TestSortCategory = "DURATION"
	TestSortCategoryTestName   TestSortCategory = "TEST_NAME"
)

var AllTestSortCategory = []TestSortCategory{
	TestSortCategoryBaseStatus,
	TestSortCategoryStatus,
	TestSortCategoryStartTime,
	TestSortCategoryDuration,
	TestSortCategoryTestName,
}

func (e TestSortCategory) IsValid() bool {
	switch e {
	case TestSortCategoryBaseStatus, TestSortCategoryStatus, TestSortCategoryStartTime, TestSortCategoryDuration, TestSortCategoryTestName:
		return true
	}
	return false
}

func (e TestSortCategory) String() string {
	return string(e)
}

func (e *TestSortCategory) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = TestSortCategory(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid TestSortCategory", str)
	}
	return nil
}

func (e TestSortCategory) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
