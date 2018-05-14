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

func (s *RoutingSuite) TestPutMethod() {
	r := s.app.AddRoute("/work").Put()
	s.Len(s.app.routes, 1)
	s.Len(r.methods, 1)
	s.Equal(r.methods[0], put)
	s.Equal(r.methods[0].String(), "PUT")
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
	s.NotEqual(hone, htwo)

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
