package gimlet

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/negroni"
)

func TestUserMiddleware(t *testing.T) {
	assert := assert.New(t)

	// set up test fixtures
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
	usermanager := &MockUserManager{Users: []*MockUser{user}}
	conf := UserMiddlewareConfiguration{}

	// make sure the constructor works right
	m := UserMiddleware(usermanager, conf)
	assert.NotNil(m)
	assert.Implements((*Middleware)(nil), m)
	assert.Implements((*negroni.Handler)(nil), m)
	assert.Equal(m.(*userMiddleware).conf, conf)
	assert.Equal(m.(*userMiddleware).manager, usermanager)

	// first test: make sure that if nothing is enabled, we pass
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
	conf = UserMiddlewareConfiguration{
		SkipHeaderCheck: false,
		SkipCookie:      true,
		HeaderUserName:  "api-user",
		HeaderKeyName:   "api-key",
	}
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
	user.Token = "42"
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
	usermanager.FailGetOrCreateUser = true
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

func TestUserMiddlewareConfiguration(t *testing.T) {
	conf := UserMiddlewareConfiguration{
		HeaderUserName: "u",
		HeaderKeyName:  "k",
		CookieName:     "c",
		CookieTTL:      time.Hour,
		CookiePath:     "/p",
	}
	require.NoError(t, conf.Validate())

	t.Run("DiabledChecksAreValid", func(t *testing.T) {
		emptyConf := UserMiddlewareConfiguration{
			SkipCookie:      true,
			SkipHeaderCheck: true,
		}
		assert.NoError(t, emptyConf.Validate())
	})
	t.Run("ZeroValueIsNotValid", func(t *testing.T) {
		emptyConf := UserMiddlewareConfiguration{}
		assert.Zero(t, emptyConf.CookiePath)
		assert.Error(t, emptyConf.Validate())
		// also we expect that the validate will populate the Tl
		assert.NotZero(t, emptyConf.CookiePath)
	})

	t.Run("Cookie", func(t *testing.T) {
		rw := httptest.NewRecorder()
		assert.Len(t, rw.Header(), 0)
		conf.AttachCookie("foo", rw)
		assert.Len(t, rw.Header(), 1)
		conf.ClearCookie(rw)
		assert.Len(t, rw.Header(), 1)
	})

	t.Run("InvalidConfigurations", func(t *testing.T) {
		for _, test := range []struct {
			name string
			op   func(UserMiddlewareConfiguration) UserMiddlewareConfiguration
		}{
			{
				name: "MissingCoookieName",
				op: func(conf UserMiddlewareConfiguration) UserMiddlewareConfiguration {
					conf.CookieName = ""
					return conf
				},
			},
			{
				name: "TooShortTTL",
				op: func(conf UserMiddlewareConfiguration) UserMiddlewareConfiguration {
					conf.CookieTTL = time.Millisecond
					return conf
				},
			},
			{
				name: "MalformedPath",
				op: func(conf UserMiddlewareConfiguration) UserMiddlewareConfiguration {
					conf.CookiePath = "foo"
					return conf
				},
			},
			{
				name: "MissingUserName",
				op: func(conf UserMiddlewareConfiguration) UserMiddlewareConfiguration {
					conf.HeaderUserName = ""
					return conf
				},
			},
			{
				name: "MissingKeyName",
				op: func(conf UserMiddlewareConfiguration) UserMiddlewareConfiguration {
					conf.HeaderKeyName = ""
					return conf
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				conf := UserMiddlewareConfiguration{
					HeaderUserName: "u",
					HeaderKeyName:  "k",
					CookieName:     "c",
					CookieTTL:      time.Hour,
					CookiePath:     "/p",
				}
				require.NoError(t, conf.Validate())
				conf = test.op(conf)
				require.Error(t, conf.Validate())
			})
		}
	})
}
