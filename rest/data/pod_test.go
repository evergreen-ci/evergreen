package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/pod"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
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

func (s *podConnectorSuite) SetupTest() {
	s.NoError(db.ClearCollections(pod.Collection))
}

func (s *podConnectorSuite) TestCreatePod() {
	p := restModel.APICreatePod{
		Name:   utility.ToStringPtr("name"),
		Memory: utility.ToIntPtr(128),
		CPU:    utility.ToIntPtr(128),
		Image:  utility.ToStringPtr("image"),
		EnvVars: []*restModel.APIPodEnvVar{
			{
				Name:   utility.ToStringPtr("env_name"),
				Value:  utility.ToStringPtr("env_value"),
				Secret: utility.ToBoolPtr(false),
			},
			{
				Name:   utility.ToStringPtr("secret_name"),
				Value:  utility.ToStringPtr("secret_value"),
				Secret: utility.ToBoolPtr(true),
			},
		},
		Platform: utility.ToStringPtr("linux"),
		Secret:   utility.ToStringPtr("secret"),
	}
	res, err := s.conn.CreatePod(p)
	s.Require().NoError(err)
	s.Require().NotZero(res)

	podDB, err := pod.FindOneByID(res.ID)
	s.Require().NoError(err)
	s.Assert().Equal("secret", podDB.Secret)
	s.Assert().Equal("env_value", podDB.TaskContainerCreationOpts.EnvVars["env_name"])
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

func (s *podConnectorSuite) TearDownTest() {
	s.NoError(db.ClearCollections(pod.Collection))
}
