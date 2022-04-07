package route

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/suite"
)

type AuthRouteSuite struct {
	suite.Suite
}

func TestAuthRouteSuite(t *testing.T) {
	s := new(AuthRouteSuite)
	suite.Run(t, s)
}

func (s *AuthRouteSuite) TestParse() {
	h := &authPermissionGetHandler{}
	urlString := "http://evergreen.mongodb.com/rest/v2/auth"
	urlString += "?resource=abc&resource_type=project&permission=read&required_level=10"
	req := &http.Request{Method: http.MethodGet}
	req.URL, _ = url.Parse(urlString)
	err := h.Parse(context.TODO(), req)
	s.Require().NoError(err)
	s.Equal("abc", h.resource)
	s.Equal("project", h.resourceType)
	s.Equal("read", h.permission)
	s.Equal(10, h.requiredLevel)
}

func (s *AuthRouteSuite) TestParseFail() {
	h := &authPermissionGetHandler{}
	urlString := "http://evergreen.mongodb.com/rest/v2/auth"
	urlString += "?resource=abc&resource_type=project&permission=read&required_level=notAnumber"
	req := &http.Request{Method: http.MethodGet}
	req.URL, _ = url.Parse(urlString)
	err := h.Parse(context.TODO(), req)
	s.Error(err)
	s.Equal("abc", h.resource)
	s.Equal("project", h.resourceType)
	s.Equal("read", h.permission)
	s.Equal(0, h.requiredLevel)
}
