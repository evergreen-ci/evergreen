package gimlet

import (
	"sync"
)

type simpleAuthenticator struct {
	mu     sync.RWMutex
	users  map[string]User
	groups map[string][]string
}

// NewSimpleAuthenticator constructs a minimum viable authenticate
// implementation, backed by access lists and user tables passed to
// the constructor. The Authenicator is, therefore, functionally
// immutable after construction.
func NewSimpleAuthenticator(users []User, groups map[string][]string) Authenticator {
	if groups == nil {
		groups = map[string][]string{}
	}

	a := &simpleAuthenticator{
		groups: groups,
		users:  map[string]User{},
	}

	for _, u := range users {
		if u != nil {
			a.users[u.Username()] = u
		}
	}

	return a
}

func (a *simpleAuthenticator) CheckResourceAccess(u User, resource string) bool {
	if !a.CheckAuthenticated(u) {
		return false
	}

	return userHasRole(u, resource)
}

func (a *simpleAuthenticator) CheckGroupAccess(u User, group string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ur, ok := a.users[u.Username()]

	if !ok {
		return false
	}

	if u.GetAPIKey() != ur.GetAPIKey() {
		return false
	}

	return userInSlice(u, a.groups[group])
}

func (a *simpleAuthenticator) CheckAuthenticated(u User) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ur, ok := a.users[u.Username()]

	if !ok {
		return false
	}

	return u.GetAPIKey() == ur.GetAPIKey()
}

type basicAuthenticator struct {
	mu        sync.RWMutex
	groups    map[string][]string
	resources map[string][]string
}

func NewBasicAuthenticator(groups, resources map[string][]string) Authenticator {
	if groups == nil {
		groups = map[string][]string{}
	}
	if resources == nil {
		resources = map[string][]string{}
	}

	return &basicAuthenticator{
		groups:    groups,
		resources: resources,
	}
}

func (a *basicAuthenticator) CheckResourceAccess(u User, resource string) bool {
	if !a.CheckAuthenticated(u) {
		return false
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	return userInSlice(u, a.resources[resource])
}

func (a *basicAuthenticator) CheckGroupAccess(u User, group string) bool {
	if !a.CheckAuthenticated(u) {
		return false
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	return userInSlice(u, a.groups[group])
}

func (a *basicAuthenticator) CheckAuthenticated(u User) bool {
	return u != nil && u.Username() != ""
}
