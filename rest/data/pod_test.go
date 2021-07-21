package data

import (
	"testing"

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

func (s *podConnectorSuite) TestCreatePod() {
	p := restModel.APICreatePod{
		Name:   utility.ToStringPtr("name"),
		Memory: utility.ToIntPtr(128),
		CPU:    utility.ToIntPtr(128),
		Image:  utility.ToStringPtr("image"),
		EnvVars: []*restModel.APIPodEnvVar{
			{
				Name:  utility.ToStringPtr("env_name"),
				Value: utility.ToStringPtr("env_value"),
			},
			{
				SecretOpts: &restModel.APISecretOpts{
					Name:  utility.ToStringPtr("secret_name"),
					Value: utility.ToStringPtr("secret_value"),
				},
			},
		},
		Platform: utility.ToStringPtr("linux"),
		Secret:   utility.ToStringPtr("secret"),
	}
	id, err := s.conn.CreatePod(p)
	s.Require().NoError(err)
	s.Require().NotZero(id)

	p = restModel.APICreatePod{}
	id, err = s.conn.CreatePod(p)
	s.Require().Error(err)
	s.Require().Zero(id)
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
