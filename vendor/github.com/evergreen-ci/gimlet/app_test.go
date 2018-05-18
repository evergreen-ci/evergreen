package gimlet

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

// AppSuite contains tests of the APIApp system. Tests of the route
// methods are ostly handled in other suites.
type AppSuite struct {
	app *APIApp
	suite.Suite
}

func TestAppSuite(t *testing.T) {
	suite.Run(t, new(AppSuite))
}

func (s *AppSuite) SetupTest() {
	s.app = NewApp()
	grip.GetSender().SetLevel(send.LevelInfo{Threshold: level.Info})
}

func (s *AppSuite) TestDefaultValuesAreSet() {
	s.Len(s.app.middleware, 2)
	s.Len(s.app.routes, 0)
	s.Equal(s.app.port, 3000)
	s.False(s.app.StrictSlash)
	s.False(s.app.isResolved)
	s.Equal(s.app.defaultVersion, -1)
}

func (s *AppSuite) TestRouterGetterReturnsErrorWhenUnresovled() {
	s.False(s.app.isResolved)

	_, err := s.app.Router()
	s.Error(err)
}

func (s *AppSuite) TestDefaultVersionSetter() {
	s.Equal(s.app.defaultVersion, -1)
	s.app.SetDefaultVersion(-2)
	s.Equal(s.app.defaultVersion, -1)

	s.app.SetDefaultVersion(0)
	s.Equal(s.app.defaultVersion, 0)

	s.app.SetDefaultVersion(1)
	s.Equal(s.app.defaultVersion, 1)

	for idx := range [100]int{} {
		s.app.SetDefaultVersion(idx)
		s.Equal(s.app.defaultVersion, idx)
	}
}

func (s *AppSuite) TestMiddleWearResetEmptiesList() {
	s.Len(s.app.middleware, 2)
	s.app.ResetMiddleware()
	s.Len(s.app.middleware, 0)
}

func (s *AppSuite) TestMiddleWearAdderAddsItemToList() {
	s.Len(s.app.middleware, 2)
	s.app.AddMiddleware(NewAppLogger())
	s.Len(s.app.middleware, 3)
}

func (s *AppSuite) TestPortSetterDoesNotAllowImpermisableValues() {
	s.Equal(s.app.port, 3000)

	for _, port := range []int{0, -1, -2000, 99999, 65536, 1000, 100, 1023} {
		err := s.app.SetPort(port)
		s.Equal(s.app.port, 3000)
		s.Error(err)
	}

	for _, port := range []int{1025, 65535, 50543, 8080, 8000} {
		err := s.app.SetPort(port)
		s.Equal(s.app.port, port)
		s.NoError(err)
	}
}

func (s *AppSuite) TestRouterReturnsRouterInstanceWhenResolved() {
	s.False(s.app.isResolved)
	r, err := s.app.Router()
	s.Nil(r)
	s.Error(err)

	s.app.AddRoute("/foo").Version(1)
	s.Error(s.app.Resolve())
	s.True(s.app.isResolved)

	r, err = s.app.Router()
	s.NotNil(r)
	s.NoError(err)
}

func (s *AppSuite) TestResolveEncountersErrorsWithAnInvalidRoot() {
	s.False(s.app.isResolved)

	s.app.AddRoute("/foo").Version(-10)
	s.Error(s.app.Resolve())
}

func (s *AppSuite) TestSetPortToExistingValueIsANoOp() {
	port := s.app.port

	s.Equal(port, s.app.port)
	s.NoError(s.app.SetPort(port))
	s.Equal(port, s.app.port)
}

func (s *AppSuite) TestResolveValidRoute() {
	s.False(s.app.isResolved)
	route := &APIRoute{
		version: 1,
		methods: []httpMethod{get},
		handler: func(_ http.ResponseWriter, _ *http.Request) { grip.Info("hello") },
		route:   "/foo",
	}
	s.True(route.IsValid())
	s.app.routes = append(s.app.routes, route)
	s.NoError(s.app.Resolve())
	s.True(s.app.isResolved)
	n, err := s.app.getNegroni()
	s.NotNil(n)
	s.NoError(err)
}

func (s *AppSuite) TestResolveAppWithDefaultVersion() {
	s.False(s.app.isResolved)
	s.app.defaultVersion = 1
	route := &APIRoute{
		version: 1,
		methods: []httpMethod{get},
		handler: func(_ http.ResponseWriter, _ *http.Request) { grip.Info("hello") },
		route:   "/foo",
	}
	s.True(route.IsValid())
	s.app.routes = append(s.app.routes, route)
	s.NoError(s.app.Resolve())
	s.True(s.app.isResolved)
}

func (s *AppSuite) TestSetHostOperations() {
	s.Equal("", s.app.address)
	s.False(s.app.isResolved)

	s.NoError(s.app.SetHost("1"))
	s.Equal("1", s.app.address)
	s.app.isResolved = true

	s.Error(s.app.SetHost("2"))
	s.Equal("1", s.app.address)
}

func (s *AppSuite) TestSetPrefix() {
	s.Equal("", s.app.prefix)

	s.app.SetPrefix("foo")
	s.Equal("/foo", s.app.prefix)
	s.app.SetPrefix("/bar")
	s.Equal("/bar", s.app.prefix)
}

func (s *AppSuite) TestGetDefaultRoute() {
	cases := map[string][]string{
		"/foo":      []string{"", "/foo"},
		"/rest/foo": []string{"", "/rest/foo"},
		"/rest/bar": []string{"/rest", "/rest/bar"},
		"/rest/baz": []string{"/rest", "/baz"},
	}

	for output, inputs := range cases {
		if !s.Len(inputs, 2) {
			continue
		}

		prefix := inputs[0]
		route := inputs[1]

		s.Equal(output, getDefaultRoute(prefix, route))
	}
}

func (s *AppSuite) TestGetVersionRoute() {
	cases := map[string][]interface{}{
		"/v1/foo":      []interface{}{"", 1, "/foo"},
		"/v1/rest/foo": []interface{}{"", 1, "/rest/foo"},
		"/rest/v2/foo": []interface{}{"/rest", 2, "/foo"},
		"/rest/v2/bar": []interface{}{"/rest", 2, "/rest/bar"},
	}
	for output, inputs := range cases {
		if !s.Len(inputs, 3) {
			continue
		}

		prefix := inputs[0].(string)
		version := inputs[1].(int)
		route := inputs[2].(string)

		s.Equal(output, getVersionedRoute(prefix, version, route))
	}
}

func (s *AppSuite) TestHandlerGetter() {
	s.NoError(s.app.Resolve())
	hone, err := s.app.getNegroni()
	s.NoError(err)
	s.NotNil(hone)
	htwo, err := s.app.Handler()
	s.NoError(err)
	s.NotNil(htwo)

	// should be equivalent results but are different instances, as each app should be distinct.
	s.NotEqual(hone, htwo)
}

func (s *AppSuite) TestAppRun() {
	s.Len(s.app.routes, 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.NoError(s.app.Resolve())
	s.NoError(s.app.Run(ctx))
}

func (s *AppSuite) TestWrapperAccessors() {
	s.Len(s.app.wrappers, 0)
	s.app.AddWrapper(NewRecoveryLogger())
	s.Len(s.app.wrappers, 1)
	s.app.RestWrappers()
	s.Len(s.app.wrappers, 0)
}
