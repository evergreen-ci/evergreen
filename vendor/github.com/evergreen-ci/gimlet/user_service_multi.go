package gimlet

import (
	"context"
	"net/http"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type multiUserManager struct {
	readWrite []UserManager
	readOnly  []UserManager
}

// NewMultiUserManager multiplexes several UserManagers into a single
// UserManager. For write operations, UserManager methods are only invoked on
// the UserManagers that can read and write. For read operations, the
// UserManager runs the method against all managers until one returns a valid
// result. Managers are prioritized in the same order in which they are passed
// into this function and read/write managers take priority over read-only
// managers.
func NewMultiUserManager(readWrite []UserManager, readOnly []UserManager) UserManager {
	return &multiUserManager{
		readWrite: readWrite,
		readOnly:  readOnly,
	}
}

func (um *multiUserManager) GetUserByToken(ctx context.Context, token string) (User, error) {
	var u User
	var err error
	if err = um.tryAllManagers(func(m UserManager) (bool, error) {
		u, err = m.GetUserByToken(ctx, token)
		return err == nil, err
	}); err != nil {
		return nil, errors.Wrap(err, "could not get user by token")
	}
	return u, nil
}

func (um *multiUserManager) CreateUserToken(username, password string) (string, error) {
	var token string
	var err error
	if err = um.tryReadWriteManagers(func(m UserManager) (bool, error) {
		token, err = m.CreateUserToken(username, password)
		return err == nil, err
	}); err != nil {
		return "", errors.Wrap(err, "could not create user token")
	}
	return token, nil
}

func (um *multiUserManager) GetLoginHandler(rootURL string) http.HandlerFunc {
	var handler http.HandlerFunc
	_ = um.tryReadWriteManagers(func(m UserManager) (bool, error) {
		handler = m.GetLoginHandler("")
		return handler == nil, nil
	})
	return handler
}

func (um *multiUserManager) GetLoginCallbackHandler() http.HandlerFunc {
	var handler http.HandlerFunc
	_ = um.tryReadWriteManagers(func(m UserManager) (bool, error) {
		handler = m.GetLoginCallbackHandler()
		return handler == nil, nil
	})
	return handler
}

func (um *multiUserManager) IsRedirect() bool {
	var isRedirect bool
	_ = um.tryReadWriteManagers(func(m UserManager) (bool, error) {
		isRedirect = m.IsRedirect()
		return isRedirect, nil
	})
	return isRedirect
}

func (um *multiUserManager) ReauthorizeUser(u User) error {
	var err error
	if err = um.tryReadWriteManagers(func(m UserManager) (bool, error) {
		err = m.ReauthorizeUser(u)
		return err == nil, err
	}); err != nil {
		return errors.Wrap(err, "could not reauthorize user")
	}
	return nil
}

func (um *multiUserManager) GetUserByID(id string) (User, error) {
	var u User
	var err error
	if err = um.tryAllManagers(func(m UserManager) (bool, error) {
		u, err = m.GetUserByID(id)
		return err == nil, err
	}); err != nil {
		return nil, errors.Wrap(err, "could not get user by ID")
	}
	return u, nil
}

func (um *multiUserManager) GetOrCreateUser(u User) (User, error) {
	var newUser User
	var err error
	if err = um.tryReadWriteManagers(func(m UserManager) (bool, error) {
		newUser, err = m.GetOrCreateUser(u)
		return err == nil, err
	}); err != nil {
		return nil, errors.Wrap(err, "could not get existing or create new user")
	}
	return newUser, nil
}

func (um *multiUserManager) ClearUser(u User, all bool) error {
	var err error
	if err = um.tryReadWriteManagers(func(m UserManager) (bool, error) {
		err = m.ClearUser(u, all)
		return err == nil, err
	}); err != nil {
		return errors.Wrap(err, "could not clear user")
	}
	return nil
}

func (um *multiUserManager) GetGroupsForUser(username string) ([]string, error) {
	var groups []string
	var err error
	if err = um.tryAllManagers(func(m UserManager) (bool, error) {
		groups, err = m.GetGroupsForUser(username)
		return err == nil, err
	}); err != nil {
		return nil, errors.Wrap(err, "could not get groups for user")
	}
	return groups, nil
}

// tryAllManagers runs a function on each managers until either managerFunc
// returns succes or it has tried and failed the operation on all managers.
func (um *multiUserManager) tryAllManagers(managerFunc func(UserManager) (success bool, err error)) error {
	return tryManagers(managerFunc, append(um.readWrite, um.readOnly...))
}

func (um *multiUserManager) tryReadWriteManagers(managerFunc func(UserManager) (success bool, err error)) error {
	return tryManagers(managerFunc, um.readWrite)
}

func tryManagers(managerFunc func(UserManager) (success bool, err error), managers []UserManager) error {
	catcher := grip.NewBasicCatcher()
	for _, m := range managers {
		success, err := managerFunc(m)
		if err == nil {
			return nil
		}
		if success {
			return nil
		}
		catcher.Add(err)
	}
	return catcher.Resolve()
}
