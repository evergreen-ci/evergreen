package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/stretchr/testify/suite"
)

type podConnectorSuite struct {
	suite.Suite
	conn Connector
}

func TestPodConnectorSuite(t *testing.T) {
	s := &podConnectorSuite{conn: &DBConnector{}}
	suite.Run(t, s)
}

func (s *podConnectorSuite) TestCheckPodSecret() {
	p := pod.Pod{
		ID:     "id",
		Secret: "secret",
	}
	s.Require().NoError(p.Insert())

	s.NoError(s.conn.CheckPodSecret("id", "secret"))
	s.Error(s.conn.CheckPodSecret("", ""))
	s.Error(s.conn.CheckPodSecret("id", ""))
	s.Error(s.conn.CheckPodSecret("id", "bad_secret"))
	s.Error(s.conn.CheckPodSecret("", "secret"))
	s.Error(s.conn.CheckPodSecret("bad_id", "secret"))
}
