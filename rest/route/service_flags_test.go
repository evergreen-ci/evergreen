package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type ServiceFlagsSuite struct {
	suite.Suite
}

func TestServiceFlagsSuite(t *testing.T) {
	suite.Run(t, new(ServiceFlagsSuite))
}

func (s *ServiceFlagsSuite) TestServiceFlagsGet() {
	ctx := context.Background()
	route := makeFetchServiceFlags().(*serviceFlagsGetHandler)

	resp := route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	flags, ok := resp.Data().(*model.APIServiceFlags)
	s.True(ok)
	s.NotNil(flags)
}
