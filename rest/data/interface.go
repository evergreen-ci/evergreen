package data

import (
	"time"

	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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

	// FindTasksByBuildId is a method to find a set of tasks which all have the same
	// BuildId. It takes the buildId being queried for as its first parameter,
	// as well as a taskId and limit for paginating through the results.
	// It returns a list of tasks which match.
	FindTasksByBuildId(string, string, string, int, int) ([]task.Task, error)

	// FindBuildById is a method to find the build matching the same BuildId.
	FindBuildById(string) (*build.Build, error)

	// FindProjectByBranch is a method to find the projectref given a branch name.
	FindProjectByBranch(string) (*model.ProjectRef, error)

	// FindByProjectAndCommit is a method to find a set of tasks which ran as part of
	// certain version in a project. It takes the projectId, commit hash, and a taskId
	// for paginating through the results.
	FindTasksByProjectAndCommit(string, string, string, string, int, int) ([]task.Task, error)

	// FindTestsByTaskId is a methodd to find a set of tests that correspond to
	// a given task. It takes a taskId, testName to start from, test status to filter,
	// limit, and sort to provide additional control over the results.
	FindTestsByTaskId(string, string, string, int, int) ([]task.TestResult, error)

	// FindUserById is a method to find a specific user given its ID.
	FindUserById(string) (auth.APIUser, error)

	// FindHostsById is a method to find a sorted list of hosts given an ID to
	// start from.
	FindHostsById(string, string, int, int) ([]host.Host, error)
	FindHostById(string) (*host.Host, error)

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
}
