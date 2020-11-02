package gimlet

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/negroni"
)

func TestUserMiddleware(t *testing.T) {
	assert := assert.New(t)

	// set up test fixtures
	//
	buf := []byte{}
	body := bytes.NewBuffer(buf)
	counter := 0
	next := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	user := &MockUser{
		ID:     "test-user",
		APIKey: "better",
	}
	usermanager := &MockUserManager{
		TokenToUsers: map[string]User{},
	}
	conf := UserMiddlewareConfiguration{}

	// make sure the constructor works right
	//
	m := UserMiddleware(usermanager, conf)
	assert.NotNil(m)
	assert.Implements((*Middleware)(nil), m)
	assert.Implements((*negroni.Handler)(nil), m)
	assert.Equal(m.(*userMiddleware).conf, conf)
	assert.Equal(m.(*userMiddleware).manager, usermanager)

	// first test: make sure that if nothing is enabled, we pass
	//
	conf = UserMiddlewareConfiguration{SkipHeaderCheck: true, SkipCookie: true}
	m = UserMiddleware(usermanager, conf)
	assert.NotNil(m)
	req := httptest.NewRequest("GET", "http://localhost/bar", body)
	rw := httptest.NewRecorder()
	m.ServeHTTP(rw, req, next)
	assert.Equal(1, counter)
	assert.Equal(http.StatusOK, rw.Code)
	rusr := GetUser(req.Context())
	assert.Nil(rusr)

	// now check if we enable one or the other or both and its not configured, there is no user attached:
	for _, conf := range []UserMiddlewareConfiguration{
		{SkipHeaderCheck: true, SkipCookie: false},
		{SkipHeaderCheck: false, SkipCookie: true},
		{SkipHeaderCheck: false, SkipCookie: false},
	} {
		m = UserMiddleware(usermanager, conf)
		assert.NotNil(m)
		req = httptest.NewRequest("GET", "http://localhost/bar", body)
		rw = httptest.NewRecorder()
		m.ServeHTTP(rw, req, next)
		assert.Equal(http.StatusOK, rw.Code)

		rusr = GetUser(req.Context())
		assert.Nil(rusr)
	}
	counter = 0

	// Check that the header check works
	//
	conf = UserMiddlewareConfiguration{
		SkipHeaderCheck: false,
		SkipCookie:      true,
		HeaderUserName:  "api-user",
		HeaderKeyName:   "api-key",
	}
	usermanager.TokenToUsers[user.ID] = user
	m = UserMiddleware(usermanager, conf)
	assert.NotNil(m)

	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	req.Header[conf.HeaderUserName] = []string{user.ID}
	req.Header[conf.HeaderKeyName] = []string{user.APIKey}
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, req, func(rw http.ResponseWriter, r *http.Request) {
		rusr := GetUser(r.Context())
		assert.Equal(user, rusr)
	})
	assert.Equal(http.StatusOK, rw.Code)

	// double check with a bad user
	req = httptest.NewRequest("GET", "http://localhost/bar", body)
	req.Header[conf.HeaderUserName] = []string{user.ID}
	req.Header[conf.HeaderKeyName] = []string{"worse"}
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, req, func(rw http.ResponseWriter, r *http.Request) {
		rusr := GetUser(r.Context())
		assert.Nil(rusr)
	})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	// check reading the cookie
	//
	conf = UserMiddlewareConfiguration{
		SkipHeaderCheck: true,
		SkipCookie:      false,
		CookieName:      "gimlet-token",
	}
	m = UserMiddleware(usermanager, conf)
	var err error
	// begin with the wrong cookie value
	req, err = http.NewRequest("GET", "http://localhost/bar", body)
	assert.NoError(err)
	req.AddCookie(&http.Cookie{
		Name:  "foo",
		Value: "better",
	})
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, req, func(rw http.ResponseWriter, r *http.Request) {
		rusr := GetUser(r.Context())
		assert.Nil(rusr)
	})
	assert.Equal(http.StatusOK, rw.Code)

	// try with the right token but wrong value
	req, err = http.NewRequest("GET", "http://localhost/bar", body)
	assert.NoError(err)
	assert.NotNil(req)
	req.AddCookie(&http.Cookie{
		Name:  "gimlet-token",
		Value: "false",
	})
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, req, func(rw http.ResponseWriter, r *http.Request) {
		rusr := GetUser(r.Context())
		assert.Nil(rusr)
	})
	assert.Equal(http.StatusOK, rw.Code)

	// try with something that should work
	usermanager.TokenToUsers["42"] = user
	req, err = http.NewRequest("GET", "http://localhost/bar", body)
	assert.NoError(err)
	assert.NotNil(req)
	req.AddCookie(&http.Cookie{
		Name:  "gimlet-token",
		Value: "42",
	})
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, req, func(rw http.ResponseWriter, r *http.Request) {
		rusr := GetUser(r.Context())
		assert.Equal(user, rusr)
	})
	assert.Equal(http.StatusOK, rw.Code)

	// test that if get-or-create fails that the op does
	usermanager.CreateUserFails = true
	req, err = http.NewRequest("GET", "http://localhost/bar", body)
	assert.NoError(err)
	assert.NotNil(req)
	req.AddCookie(&http.Cookie{
		Name:  "gimlet-token",
		Value: "42",
	})
	rw = httptest.NewRecorder()
	counter = 0
	m.ServeHTTP(rw, req, func(rw http.ResponseWriter, r *http.Request) {
		rusr := GetUser(r.Context())
		assert.Nil(rusr)
	})
	assert.Equal(http.StatusOK, rw.Code)
}
