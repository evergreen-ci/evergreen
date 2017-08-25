package data

import (
	"time"

	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/grip/message"
)

// Connector is an interface that contains all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type Connector interface {
	// Get and Set SuperUsers provide access to the list of API super users.
	GetSuperUsers() []string
	SetSuperUsers([]string)

	// Get and Set URL provide access to the main url string of the API.
	GetURL() string
	SetURL(string)

	// Get and Set Prefix provide access to the prefix that prepends all of the
	// URL paths.
	GetPrefix() string
	SetPrefix(string)

	// FindTaskById is a method to find a specific task given its ID.
	FindTaskById(string) (*task.Task, error)
	FindTasksByIds([]string) ([]task.Task, error)
	SetTaskPriority(*task.Task, int64) error
	SetTaskActivated(string, string, bool) error
	ResetTask(string, string, *model.Project) error
	AbortTask(string, string) error

	// FindTasksByBuildId is a method to find a set of tasks which all have the same
	// BuildId. It takes the buildId being queried for as its first parameter,
	// as well as a taskId and limit for paginating through the results.
	// It returns a list of tasks which match.
	FindTasksByBuildId(string, string, string, int, int) ([]task.Task, error)

	// FindBuildById is a method to find the build matching the same BuildId.
	FindBuildById(string) (*build.Build, error)
	// SetBuildPriority and SetBuildActivated change the status of the input build
	SetBuildPriority(string, int64) error
	SetBuildActivated(string, string, bool) error

	// AbortBuild is a method to abort the build matching the same BuildId.
	AbortBuild(string, string) error
	// RestartBuild is a method to restart the build matching the same BuildId.
	RestartBuild(string, string) error

	// FindProjects is a method to find projects as ordered by name
	FindProjects(string, int, int, bool) ([]model.ProjectRef, error)
	// FindProjectVars is a method to fetch the vars for a given project
	FindProjectVars(string) (*model.ProjectVars, error)
	// FindProjectByBranch is a method to find the projectref given a branch name.
	FindProjectByBranch(string) (*model.ProjectRef, error)

	// FindByProjectAndCommit is a method to find a set of tasks which ran as part of
	// certain version in a project. It takes the projectId, commit hash, and a taskId
	// for paginating through the results.
	FindTasksByProjectAndCommit(string, string, string, string, int, int) ([]task.Task, error)

	// FindTestsByTaskId is a method to find a set of tests that correspond to
	// a given task. It takes a taskId, testName to start from, test status to filter,
	// limit, and sort to provide additional control over the results.
	FindTestsByTaskId(string, string, string, int, int) ([]task.TestResult, error)

	// FindUserById is a method to find a specific user given its ID.
	FindUserById(string) (auth.APIUser, error)

	// FindHostsById is a method to find a sorted list of hosts given an ID to
	// start from.
	FindHostsById(string, string, string, int, int) ([]host.Host, error)
	FindHostById(string) (*host.Host, error)

	// NewIntentHost is a method to insert an intent host given a distro and the name of a saved public key
	NewIntentHost(string, string, *user.DBUser) (*host.Host, error)

	// FetchContext is a method to fetch a context given a series of identifiers.
	FetchContext(string, string, string, string, string) (model.Context, error)

	// FindAllDistros is a method to find a sorted list of all distros.
	FindAllDistros() ([]distro.Distro, error)

	// FindTaskSystemMetrics and FindTaskProcessMetrics provide
	// access to the metrics data collected by agents during task execution
	FindTaskSystemMetrics(string, time.Time, int, int) ([]*message.SystemInfo, error)
	FindTaskProcessMetrics(string, time.Time, int, int) ([][]*message.ProcessInfo, error)

	// FindCostByVersionId returns cost data of a version given its ID.
	FindCostByVersionId(string) (*task.VersionCost, error)

	// FindCostByDistroId returns cost data of a distro given its ID and a time range.
	// Interested time range is given as a start time and duration.
	FindCostByDistroId(string, time.Time, time.Duration) (*task.DistroCost, error)

	// FindVersionById returns version given its ID.
	FindVersionById(string) (*version.Version, error)

	// FindPatchesByProject provides access to the patches corresponding to the input project ID
	// as ordered by creation time.
	FindPatchesByProject(string, time.Time, int, bool) ([]patch.Patch, error)
	// FindPatchByUser finds patches for the input user as ordered by creation time
	FindPatchesByUser(string, time.Time, int, bool) ([]patch.Patch, error)

	// FindPatchById fetches the patch corresponding to the input patch ID.
	FindPatchById(string) (*patch.Patch, error)

	// AbortVersion aborts all tasks of a version given its ID.
	AbortVersion(string) error

	// AbortPatch aborts the patch corresponding to the input patch ID and deletes if not finalized.
	AbortPatch(string, string) error

	// RestartVersion restarts all completed tasks of a version given its ID and the caller.
	RestartVersion(string, string) error
	// SetPatchPriority and SetPatchActivated change the status of the input patch
	SetPatchPriority(string, int64) error
	SetPatchActivated(string, string, bool) error

	// GetAdminSettings/SetAdminSettings retrieves/sets the system-wide settings document
	GetAdminSettings() (*admin.AdminSettings, error)
	SetAdminSettings(*admin.AdminSettings) error
	// SetAdminBanner sets set the banner in the system-wide settings document
	SetAdminBanner(string) error
	// SetAdminBanner sets set the service flags in the system-wide settings document
	SetServiceFlags(admin.ServiceFlags) error

	FindCostTaskByProject(string, string, time.Time, time.Time, int, int) ([]task.Task, error)
}
