package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
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

func (s *podConnectorSuite) TearDownTest() {
	s.NoError(db.ClearCollections(pod.Collection))
}

func (s *podConnectorSuite) TestCreatePod() {
	p := model.APICreatePod{
		Name:   utility.ToStringPtr("name"),
		Memory: utility.ToIntPtr(128),
		CPU:    utility.ToIntPtr(128),
		Image:  utility.ToStringPtr("image"),
		EnvVars: []*model.APIPodEnvVar{
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
		OS:     utility.ToStringPtr("linux"),
		Arch:   utility.ToStringPtr("amd64"),
		Secret: utility.ToStringPtr("secret"),
	}
	res, err := s.conn.CreatePod(p)
	s.Require().NoError(err)
	s.Require().NotZero(res)

	dbPod, err := pod.FindOneByID(res.ID)
	s.Require().NoError(err)
	s.Equal("secret", dbPod.Secret)
	s.Require().NotZero(dbPod.TaskContainerCreationOpts.EnvVars)
	s.Equal("env_value", dbPod.TaskContainerCreationOpts.EnvVars["env_name"])
	s.NotZero(dbPod.TaskContainerCreationOpts.EnvVars["POD_ID"])
	s.Require().NotZero(dbPod.TaskContainerCreationOpts.EnvSecrets)
	s.Equal(utility.FromStringPtr(p.Secret), dbPod.TaskContainerCreationOpts.EnvSecrets["POD_SECRET"])
}

func (s *podConnectorSuite) TestFindPodByIDSucceeds() {
	p := pod.Pod{
		ID:     "id",
		Secret: "secret",
		Status: pod.StatusRunning,
	}
	s.Require().NoError(p.Insert())
	apiPod, err := s.conn.FindPodByID(p.ID)
	s.Require().NoError(err)
	s.Equal(p.ID, utility.FromStringPtr(apiPod.ID))
	s.Equal(p.Secret, utility.FromStringPtr(apiPod.Secret))
}

func (s *podConnectorSuite) TestFindPodByIDFailsWithNonexistentPod() {
	apiPod, err := s.conn.FindPodByID("nonexistent")
	s.Error(err)
	s.Zero(apiPod)
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
