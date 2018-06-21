package gimlet

import "sync"

type basicAuthenticator struct {
	mu     sync.RWMutex
	users  map[string]User
	groups map[string][]string
}

func NewBasicAuthenticator(users []User, groups map[string][]string) Authenticator {
	if groups == nil {
		groups = map[string][]string{}
	}

	a := &basicAuthenticator{
		groups: groups,
		users:  map[string]User{},
	}

	for _, u := range users {
		if u != nil && !u.IsNil() {
			a.users[u.Username()] = u
		}
	}

	return a
}

func (a *basicAuthenticator) CheckResourceAccess(u User, resource string) bool {
	if !a.CheckAuthenticated(u) {
		return false
	}

	return userHasRole(u, resource)
}

func (a *basicAuthenticator) CheckGroupAccess(u User, group string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ur, ok := a.users[u.Username()]

	if !ok {
		return false
	}

	if u.GetAPIKey() != ur.GetAPIKey() {
		return false
	}

	return userInGroup(u, a.groups[group])
}

func (a *basicAuthenticator) CheckAuthenticated(u User) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ur, ok := a.users[u.Username()]

	if !ok {
		return false
	}

	return u.GetAPIKey() == ur.GetAPIKey()
}
