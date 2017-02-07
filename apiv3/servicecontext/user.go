package servicecontext

import (
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model/user"
)

// DBUserConnector is a struct that implements the User related interface
// of the ServiceContext interface through interactions with the backing database.
type DBUserConnector struct{}

// FindUserById uses the service layer's user type to query the backing database for
// the user with the given Id.
func (tc *DBUserConnector) FindUserById(userId string) (auth.APIUser, error) {
	t, err := user.FindOne(user.ById(userId))
	if err != nil {
		return nil, err
	}
	return t, nil
}

// MockUserConnector stores a cached set of users that are queried against by the
// implementations of the UserConnector interface's functions.
type MockUserConnector struct {
	CachedUsers map[string]*user.DBUser
}

// FindUserById provides a mock implementation of the User functions
// from the ServiceContext that does not need to use a database.
// It returns results based on the cached users in the MockUserConnector.
func (muc *MockUserConnector) FindUserById(userId string) (auth.APIUser, error) {
	u := muc.CachedUsers[userId]
	return u, nil
}
