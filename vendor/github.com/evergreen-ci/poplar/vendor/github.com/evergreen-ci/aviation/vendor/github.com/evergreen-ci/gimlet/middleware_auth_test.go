package gimlet

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMiddlewareValueAccessors(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	a := GetAuthenticator(ctx)
	assert.Nil(a)

	userm := GetUserManager(ctx)
	assert.Nil(userm)

	usr := GetUser(ctx)
	assert.Nil(usr)

	var idone, idtwo int
	idone = getNumber()
	assert.Equal(0, idtwo)
	assert.True(idone > 0)
	assert.NotPanics(func() { idtwo = GetRequestID(ctx) })
	assert.True(idone > idtwo)
	assert.Equal(-1, idtwo)

	// some gross checks to make sure that we're safe if the authenticator is the wrong type
	ctx = context.WithValue(ctx, authHandlerKey, true)
	ctx = context.WithValue(ctx, userManagerKey, true)
	ctx = context.WithValue(ctx, userKey, true)

	a = GetAuthenticator(ctx)
	assert.Nil(a)

	userm = GetUserManager(ctx)
	assert.Nil(userm)

	usr = GetUser(ctx)
	assert.Nil(usr)
}

func TestAuthMiddlewareConstructors(t *testing.T) {
	assert := assert.New(t) // nolint
	authenticator := &MockAuthenticator{}
	usermanager := &MockUserManager{}

	ah, ok := NewAuthenticationHandler(authenticator, usermanager).(*authHandler)
	assert.True(ok)
	assert.NotNil(ah)
	assert.Equal(authenticator, ah.auth)
	assert.Equal(usermanager, ah.um)

	ra, ok := NewRoleRequired("foo").(*requiredRole)
	assert.True(ok)
	assert.NotNil(ra)
	assert.Equal("foo", ra.role)

	rg, ok := NewGroupMembershipRequired("foo").(*requiredGroup)
	assert.True(ok)
	assert.NotNil(rg)
	assert.Equal("foo", rg.group)

	rah, ok := NewRequireAuthHandler().(*requireAuthHandler)
	assert.True(ok)
	assert.NotNil(rah)
}

func TestAuthRequiredBehavior(t *testing.T) {
	assert := assert.New(t) // nolint
	buf := []byte{}
	body := bytes.NewBuffer(buf)

	counter := 0
	next := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}

	authenticator := &MockAuthenticator{
		UserToken:               "test",
		CheckAuthenticatedState: map[string]bool{},
	}
	user := &MockUser{
		ID: "test-user",
	}
	baduser := &MockUser{
		ID: "bad-user",
	}
	usermanager := &MockUserManager{
		TokenToUsers: map[string]User{},
	}

	ra := NewRequireAuthHandler()

	// start without any context setup
	//
	req := httptest.NewRequest("GET", "http://localhost/bar", body)
	rw := httptest.NewRecorder()
	ra.ServeHTTP(rw, req, next)

	// there's nothing attached to the context, so it's a 401
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try again with an authenticator...
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx := req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)

	// just the authenticator isn't enough
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try with a user manager
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	ra.ServeHTTP(rw, req, next)

	// just the authenticator isn't users aren't enough
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// now set up the user
	//
	usermanager.TokenToUsers[authenticator.UserToken] = user

	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)

	// shouldn't work because the authenticator doesn't have the user registered
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// register a bad user
	//
	authenticator.CheckAuthenticatedState[user.Username()] = true
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	req = setUserForRequest(req, baduser)

	ra.ServeHTTP(rw, req, next)

	// register the correct user
	//
	authenticator.CheckAuthenticatedState[user.Username()] = true
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	req = setUserForRequest(req, user)

	ra.ServeHTTP(rw, req, next)

	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)
}

func TestAuthAttachWrapper(t *testing.T) {
	assert := assert.New(t) // nolint
	buf := []byte{}
	body := bytes.NewBuffer(buf)

	req := httptest.NewRequest("GET", "http://localhost/bar", body)
	rw := httptest.NewRecorder()
	counter := 0
	authenticator := &MockAuthenticator{
		UserToken:               "test",
		CheckAuthenticatedState: map[string]bool{},
	}
	usermanager := &MockUserManager{
		TokenToUsers: map[string]User{},
	}

	ah := NewAuthenticationHandler(authenticator, usermanager)
	assert.NotNil(ah)
	assert.Equal(ah.(*authHandler).um, usermanager)
	assert.Equal(ah.(*authHandler).auth, authenticator)

	baseCtx := context.Background()
	req = req.WithContext(baseCtx)

	assert.Exactly(req.Context(), baseCtx)

	ah.ServeHTTP(rw, req, func(nrw http.ResponseWriter, r *http.Request) {
		rctx := r.Context()
		assert.NotEqual(rctx, baseCtx)

		um := GetUserManager(rctx)
		assert.Equal(usermanager, um)

		ath := GetAuthenticator(rctx)
		assert.Equal(authenticator, ath)

		counter++
		nrw.WriteHeader(http.StatusTeapot)
	})

	assert.Equal(1, counter)
	assert.Equal(http.StatusTeapot, rw.Code)
}

func TestRoleRestrictedAccessMiddleware(t *testing.T) {
	assert := assert.New(t) // nolint
	buf := []byte{}
	body := bytes.NewBuffer(buf)

	counter := 0
	next := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}

	authenticator := &MockAuthenticator{
		UserToken:               "test",
		CheckAuthenticatedState: map[string]bool{},
		GroupUserMapping:        map[string]string{},
	}
	user := &MockUser{
		ID:        "test-user",
		RoleNames: []string{"staff"},
	}
	usermanager := &MockUserManager{
		TokenToUsers: map[string]User{},
	}

	ra := NewRoleRequired("sudo")

	// start without any context setup
	//
	req := httptest.NewRequest("GET", "http://localhost/bar", body)
	rw := httptest.NewRecorder()
	ra.ServeHTTP(rw, req, next)

	// there's nothing attached to the context, so it's a 401
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try again with an authenticator...
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx := req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)

	// just the authenticator isn't enough
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try with a user manager
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	ra.ServeHTTP(rw, req, next)

	// just the authenticator isn't users aren't enough
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// now set up the user (which is defined with the wrong role, so won't work)
	//
	usermanager.TokenToUsers[authenticator.UserToken] = user

	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	req = setUserForRequest(req, user)

	ra.ServeHTTP(rw, req, next)

	// shouldn't work because the authenticator doesn't have the user registered
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// make the user have the right access
	//
	user.RoleNames = []string{"sudo"}

	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	req = setUserForRequest(req, user)

	ra.ServeHTTP(rw, req, next)

	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)
}

func TestGroupAccessRequired(t *testing.T) {
	assert := assert.New(t) // nolint
	buf := []byte{}
	body := bytes.NewBuffer(buf)

	counter := 0
	next := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}

	authenticator := &MockAuthenticator{
		UserToken:               "test",
		CheckAuthenticatedState: map[string]bool{},
		GroupUserMapping: map[string]string{
			"test-user": "staff",
		},
	}
	user := &MockUser{
		ID: "test-user",
	}
	usermanager := &MockUserManager{
		TokenToUsers: map[string]User{},
	}

	ra := NewGroupMembershipRequired("sudo")

	// start without any context setup
	//
	req := httptest.NewRequest("GET", "http://localhost/bar", body)
	rw := httptest.NewRecorder()
	ra.ServeHTTP(rw, req, next)

	// there's nothing attached to the context, so it's a 401
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try again with an authenticator...
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx := req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)

	// just the authenticator isn't enough
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try with a user manager
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	ra.ServeHTTP(rw, req, next)

	// just the authenticator isn't users aren't enough
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// now set up the user (which has the wrong access defined)
	//
	usermanager.TokenToUsers[authenticator.UserToken] = user

	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	req = setUserForRequest(req, user)

	ra.ServeHTTP(rw, req, next)

	// shouldn't work because the authenticator doesn't have the user registered
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// make the user have the right access
	//
	authenticator.GroupUserMapping["test-user"] = "sudo"

	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = setUserManager(ctx, usermanager)
	req = req.WithContext(ctx)
	req = setUserForRequest(req, user)

	ra.ServeHTTP(rw, req, next)

	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)
}

func TestRestrictedAccessMiddleware(t *testing.T) {
	assert := assert.New(t)
	buf := []byte{}
	body := bytes.NewBuffer(buf)

	counter := 0
	next := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}

	authenticator := &MockAuthenticator{
		UserToken: "test",
		CheckAuthenticatedState: map[string]bool{
			"test-user": true,
		},
		GroupUserMapping: map[string]string{
			"test-user": "staff",
		},
	}
	user := &MockUser{
		ID: "test-user",
	}
	baduser := &MockUser{
		ID: "prod-user",
	}

	ra := NewRestrictAccessToUsers([]string{"test-user"})

	// start without any context setup
	//
	req := httptest.NewRequest("GET", "http://localhost/bar", body)
	rw := httptest.NewRecorder()
	ra.ServeHTTP(rw, req, next)

	// there's nothing attached to the context, so it's a 401
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try again with a user attached but no authenticator...
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx := req.Context()
	ctx = context.WithValue(ctx, userKey, user)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try with the wrong user, that can't authenticate
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = context.WithValue(ctx, userKey, baduser)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try with the wrong user that can authenticate but isn't allowed
	//
	authenticator.CheckAuthenticatedState[baduser.ID] = true
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = context.WithValue(ctx, userKey, baduser)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// try with the correct user
	//
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	rw = httptest.NewRecorder()
	ctx = req.Context()
	ctx = setAuthenticator(ctx, authenticator)
	ctx = context.WithValue(ctx, userKey, user)
	req = req.WithContext(ctx)

	ra.ServeHTTP(rw, req, next)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)

}
