package servicecontext

import (
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
)

// ServiceContext is an interface that contains all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type ServiceContext interface {
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

	// FetchContext is a method to fetch a context given a series of identifiers.
	FetchContext(string, string, string, string, string) (model.Context, error)
}

// DBServiceContext is a struct that implements all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type DBServiceContext struct {
	superUsers []string
	URL        string
	Prefix     string

	DBUserConnector
	DBTaskConnector
	DBContextConnector
	DBHostConnector
	DBTestConnector
}

func (ctx *DBServiceContext) GetSuperUsers() []string {
	return ctx.superUsers
}
func (ctx *DBServiceContext) SetSuperUsers(su []string) {
	ctx.superUsers = su
}
func (ctx *DBServiceContext) GetURL() string {
	return ctx.URL
}
func (ctx *DBServiceContext) SetURL(url string) {
	ctx.URL = url
}
func (ctx *DBServiceContext) GetPrefix() string {
	return ctx.Prefix
}
func (ctx *DBServiceContext) SetPrefix(prefix string) {
	ctx.Prefix = prefix
}

type MockServiceContext struct {
	superUsers []string
	URL        string
	Prefix     string

	MockUserConnector
	MockTaskConnector
	MockContextConnector
	MockHostConnector
	MockTestConnector
}

func (ctx *MockServiceContext) GetSuperUsers() []string {
	return ctx.superUsers
}
func (ctx *MockServiceContext) SetSuperUsers(su []string) {
	ctx.superUsers = su
}
func (ctx *MockServiceContext) GetURL() string {
	return ctx.URL
}
func (ctx *MockServiceContext) SetURL(url string) {
	ctx.URL = url
}
func (ctx *MockServiceContext) GetPrefix() string {
	return ctx.Prefix
}
func (ctx *MockServiceContext) SetPrefix(prefix string) {
	ctx.Prefix = prefix
}
