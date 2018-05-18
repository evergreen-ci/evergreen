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
	s.Len(s.app.middleware, 3)
	s.Len(s.app.routes, 0)
	s.Equal(s.app.port, 3000)
	s.True(s.app.StrictSlash)
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
	s.Len(s.app.middleware, 3)
	s.app.ResetMiddleware()
	s.Len(s.app.middleware, 0)
}

func (s *AppSuite) TestMiddleWearAdderAddsItemToList() {
	s.Len(s.app.middleware, 3)
	s.app.AddMiddleware(NewAppLogger())
	s.Len(s.app.middleware, 4)
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

func (s *AppSuite) TestAddAppReturnsErrorIfOuterAppIsResolved() {
	newApp := NewApp()
	err := newApp.Resolve()
	s.NoError(err)
	s.True(newApp.isResolved)

	// if you attempt use AddApp on an app that is already
	// resolved, it returns an error.
	s.Error(newApp.AddApp(s.app))
}

func (s *AppSuite) TestAddAppReturnsNoErrorIfInnerAppIsResolved() {
	newApp := NewApp()
	err := s.app.Resolve()
	s.NoError(err)
	s.True(s.app.isResolved)

	s.NoError(newApp.AddApp(s.app))
}

func (s *AppSuite) TestRouteMergingInIfVersionsAreTheSame() {
	subApp := NewApp()
	s.Len(subApp.routes, 0)
	route := subApp.AddRoute("/foo")
	s.Len(subApp.routes, 1)

	s.Len(s.app.subApps, 0)
	err := s.app.AddApp(subApp)
	s.NoError(err)
	s.Len(s.app.subApps, 1)
	s.Equal(s.app.subApps[0].routes[0], route)
}

func (s *AppSuite) TestRouteMergingInWithDifferntVersions() {
	// If the you have two apps with different default versions,
	// routes in the sub-app that don't have a version set, should
	// get their version set to whatever the value of the sub
	// app's default value at the time of merging the apps.
	subApp := NewApp()
	subApp.SetDefaultVersion(2)
	s.NotEqual(s.app.defaultVersion, subApp.defaultVersion)

	// add a route to the first app
	s.Len(subApp.routes, 0)
	route := subApp.AddRoute("/foo").Version(3)
	s.Equal(route.version, 3)
	s.Len(subApp.routes, 1)

	// try adding to second app, to the first, with one route
	s.Len(s.app.subApps, 0)
	err := s.app.AddApp(subApp)
	s.NoError(err)
	s.Len(s.app.subApps, 1)
	s.Equal(s.app.subApps[0].routes[0], route)

	nextApp := NewApp()
	s.Len(nextApp.routes, 0)
	nextRoute := nextApp.AddRoute("/bar")
	s.Len(nextApp.routes, 1)
	s.Equal(nextRoute.version, -1)
	nextApp.SetDefaultVersion(3)
	s.Equal(nextRoute.version, -1)

	// make sure the default value of nextApp is on the route in the subApp
	err = s.app.AddApp(nextApp)
	s.NoError(err)
	s.Equal(s.app.subApps[1].routes[0], nextRoute)
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

func (s *AppSuite) TestSubAppResolution() {
	s.False(s.app.isResolved)
	route := &APIRoute{
		version: 1,
		methods: []httpMethod{get},
		handler: func(_ http.ResponseWriter, _ *http.Request) { grip.Info("hello") },
		route:   "/foo",
	}
	s.True(route.IsValid())
	s.app.routes = append(s.app.routes, route)
	subApp := NewApp()
	s.app.AddApp(subApp)

	n, err := s.app.getNegroni()
	s.NotNil(n)
	s.NoError(err)
}

func (s *AppSuite) TestSubAppResolutionWithErrors() {
	s.False(s.app.isResolved)
	route := &APIRoute{
		version: 1,
		methods: []httpMethod{get},
		handler: func(_ http.ResponseWriter, _ *http.Request) { grip.Info("hello") },
		route:   "/foo",
	}
	s.True(route.IsValid())
	s.app.routes = append(s.app.routes, route)
	s.app.SetPrefix("/rest")

	badRoute := &APIRoute{
		version: -1,
		methods: []httpMethod{get},
		handler: func(_ http.ResponseWriter, _ *http.Request) { grip.Info("hello") },
		route:   "",
	}
	s.False(badRoute.IsValid())

	subApp := NewApp()
	subApp.routes = append(subApp.routes, badRoute)

	s.app.AddApp(subApp)

	n, err := s.app.getNegroni()
	s.Nil(n)
	s.Error(err)
	s.Error(s.app.Run(context.Background()))
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
	s.NoError(s.app.Run(ctx))
}
