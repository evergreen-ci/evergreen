package route

import (
	"context"
	"encoding/json"
	"testing"

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
	s.Equal(200, resp.Status())

	data, err := json.Marshal(resp.Data())
	s.NoError(err)
	s.JSONEq(`{"static_api_keys_disabled":true}`, string(data))
}
