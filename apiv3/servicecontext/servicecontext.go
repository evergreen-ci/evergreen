package servicecontext

import (
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
)

// ServiceContext is an interface that contains all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type ServiceContext interface {
	GetSuperUsers() []string
	SetSuperUsers([]string)

	// FindTaskById is a method to find a specific task given its ID.
	FindTaskById(string) (*task.Task, error)

	// FindTasksByBuildId is a method to find a set of tasks which all have the same
	// BuildId. It takes the buildId being queried for as its parameter and
	// returns a list of tasks which match.
	FindTasksByBuildId(string, string, int) ([]task.Task, error)

	// FindUserById is a method to find a specific user given its ID.
	FindUserById(string) (auth.APIUser, error)

	// FetchContext is a method to fetch a context given a series of identifiers.
	FetchContext(string, string, string, string, string) (model.Context, error)
}

// DBServiceContext is a struct that implements all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type DBServiceContext struct {
	superUsers []string

	DBUserConnector
	DBTaskConnector
	DBContextConnector
}

func (ctx *DBServiceContext) GetSuperUsers() []string {
	return ctx.superUsers
}

func (ctx *DBServiceContext) SetSuperUsers(su []string) {
	ctx.superUsers = su
}

type MockServiceContext struct {
	superUsers []string

	MockUserConnector
	MockTaskConnector
	MockContextConnector
}

func (ctx *MockServiceContext) GetSuperUsers() []string {
	return ctx.superUsers
}
func (ctx *MockServiceContext) SetSuperUsers(su []string) {
	ctx.superUsers = su
}
