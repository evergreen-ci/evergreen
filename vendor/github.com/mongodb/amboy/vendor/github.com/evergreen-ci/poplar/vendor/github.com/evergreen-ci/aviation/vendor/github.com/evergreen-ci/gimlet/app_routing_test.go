package gimlet

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mongodb/grip"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RoutingSuite struct {
	app     *APIApp
	require *require.Assertions
	suite.Suite
}

func TestRoutingSuite(t *testing.T) {
	suite.Run(t, new(RoutingSuite))
}

func (s *RoutingSuite) SetupSuite() {
	s.require = s.Require()
}

func (s *RoutingSuite) SetupTest() {
	s.app = NewApp()
}

func (s *RoutingSuite) TestRouteConstructorAlwaysAddsPrefix() {
	for idx, n := range []string{"foo", "bar", "baz", "f", "e1"} {
		s.True(!strings.HasPrefix(n, "/"))
		r := s.app.AddRoute(n)
		s.Len(s.app.routes, idx+1)
		s.True(strings.HasPrefix(r.route, "/"), r.route)
	}
}

func (s *RoutingSuite) TestRoutePrefixConstructor() {
	one := s.app.PrefixRoute("foo").Route("baz")
	two := s.app.AddRoute("baz").Prefix("foo")
	s.Equal(one.prefix, two.prefix)
	s.Equal(one.route, two.route)
	s.Equal(one, two)
}

func (s *RoutingSuite) TestPutMethod() {
	r := s.app.AddRoute("/work").Put()
	s.Len(s.app.routes, 1)
	s.Len(r.methods, 1)
	s.Equal(r.methods[0], put)
	s.Equal(r.methods[0].String(), "PUT")
}

func (s *RoutingSuite) TestPrefixMethod() {
	r := s.app.AddRoute("/work").Prefix("foo")
	s.Len(s.app.routes, 1)
	s.Equal(r, s.app.routes[0])
	s.Equal(s.app.routes[0].prefix, "/foo")

	s.False(r.overrideAppPrefix)
	r.OverridePrefix()
	s.True(r.overrideAppPrefix)

}

func (s *RoutingSuite) TestDeleteMethod() {
	r := s.app.AddRoute("/work").Delete()
	s.Len(s.app.routes, 1)
	s.Len(r.methods, 1)
	s.Equal(r.methods[0], delete)
	s.Equal(r.methods[0].String(), "DELETE")
}

func (s *RoutingSuite) TestGetMethod() {
	r := s.app.AddRoute("/work").Get()
	s.Len(s.app.routes, 1)
	s.Len(r.methods, 1)
	s.Equal(r.methods[0], get)
	s.Equal(r.methods[0].String(), "GET")
}

func (s *RoutingSuite) TestPatchMethod() {
	r := s.app.AddRoute("/work").Patch()
	s.Len(s.app.routes, 1)
	s.Len(r.methods, 1)
	s.Equal(r.methods[0], patch)
	s.Equal(r.methods[0].String(), "PATCH")
}

func (s *RoutingSuite) TestPostMethod() {
	r := s.app.AddRoute("/work").Post()
	s.Len(s.app.routes, 1)
	s.Len(r.methods, 1)
	s.Equal(r.methods[0], post)
	s.Equal(r.methods[0].String(), "POST")
}

func (s *RoutingSuite) TestHeadMethod() {
	r := s.app.AddRoute("/work").Head()
	s.Len(s.app.routes, 1)
	s.Len(r.methods, 1)
	s.Equal(r.methods[0], head)
	s.Equal(r.methods[0].String(), "HEAD")
}

func (s *RoutingSuite) TestRouteHandlerInterface() {
	s.Len(s.app.routes, 0)
	r := s.app.AddRoute("/foo")
	s.NotNil(r)
	s.Nil(r.handler)
	s.Len(s.app.routes, 1)
	s.Equal(r, s.app.routes[0])
	r.RouteHandler(nil)
	s.NotNil(r.handler)
	r.RouteHandler(nil)
	s.NotNil(r.handler)
}

func (s *RoutingSuite) TestHandlerMethod() {
	s.Len(s.app.routes, 0)
	r := s.app.AddRoute("/foo")
	s.Len(s.app.routes, 1)
	s.NotNil(r)
	s.Nil(r.handler)
	r.Handler(nil)
	s.Nil(r.handler)

	hone := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { grip.Debug("dummy route") })
	htwo := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { grip.Debug("dummy route two") })
	s.NotEqual(fmt.Sprint(hone), fmt.Sprint(htwo))

	r.Handler(hone)
	s.Equal(fmt.Sprint(r.handler), fmt.Sprint(hone))

	r.Handler(htwo)
	s.Equal(fmt.Sprint(r.handler), fmt.Sprint(htwo))
}

func (s *RoutingSuite) TestRouteValidation() {
	r := &APIRoute{}
	s.False(r.IsValid())

	r.version = 2
	s.False(r.IsValid())

	r.methods = []httpMethod{get, delete, post}
	s.False(r.IsValid())

	r.handler = func(_ http.ResponseWriter, _ *http.Request) { grip.Debug("dummy route") }
	s.False(r.IsValid())

	r.route = "/foo"
	s.True(r.IsValid())
}

func (s *RoutingSuite) TestStringNoMethods() {
	r := s.app.Route().Route("/foo").Version(2)
	str := r.String()
	s.Contains(str, "v='2")
	s.Contains(str, "/foo")
	s.Contains(str, "defined=false")
	s.Contains(str, "defined=false")
	s.Contains(str, "methods=[]")
}

func (s *RoutingSuite) TestStringWithMethods() {
	r := s.app.AddRoute("/foo").Version(2).Get().Patch()
	str := r.String()
	s.Contains(str, "v='2")
	s.Contains(str, "/foo")
	s.Contains(str, "defined=false")
	s.Contains(str, "defined=false")
	s.Contains(str, "GET")
	s.Contains(str, "PATCH")
}

func (s *RoutingSuite) TestLegacyRouteResolution() {
	r := s.app.PrefixRoute("/foo").Route("/bar")

	out := r.resolveLegacyRoute(s.app, false)
	s.Equal("/foo/bar", out)

	r = s.app.Route().Route("/foo/bar")

	out = r.resolveLegacyRoute(s.app, false)
	s.Equal("/foo/bar", out)

}

func (s *RoutingSuite) TestResolveRoutes() {
	s.app.prefix = "/bar"
	r := s.app.AddRoute("/foo").Version(-1).Get()
	s.Equal("/foo", r.resolveLegacyRoute(s.app, false))
	s.Equal("/bar/foo", r.resolveLegacyRoute(s.app, true))

	r.route = "/foo"
	s.Equal("/foo", r.resolveLegacyRoute(s.app, false))
	s.Equal("/bar/foo", r.resolveLegacyRoute(s.app, true))
}

func (s *RoutingSuite) TestMethodMethodValidCases() {
	methods := []string{"GET", "PUT", "POST", "DELETE", "PATCH", "HEAD"}
	for idx, m := range methods {
		s.Len(s.app.routes, idx)
		r := s.app.AddRoute("/" + m + "Foo").Method(m)
		s.Len(r.methods, 1)
		s.NotNil(r)
	}
}

func (s *RoutingSuite) TestMethodMethodIsInvalid() {
	methods := []string{"FFFF", "PPPPP", "RRRR", "DELTER", "PUTCH", "TRACE"}
	for idx, m := range methods {
		s.Len(s.app.routes, idx)
		r := s.app.AddRoute("/" + m + "Foo").Method(m)
		s.Len(r.methods, 0)
		s.NotNil(r)
	}
}

func (s *RoutingSuite) TestRouteWrapperMutators() {
	r := s.app.AddRoute("/foo")
	s.Len(r.wrappers, 0)
	r.Wrap(MakeRecoveryLogger())
	s.Len(r.wrappers, 1)
	r.ClearWrappers()
	s.Len(r.wrappers, 0)
}
