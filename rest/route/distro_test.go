package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch distro by id

type DistroByIdSuite struct {
	sc   *data.MockConnector
	data data.MockDistroConnector
	rm   gimlet.RouteHandler

	suite.Suite
}

func TestDistroSuite(t *testing.T) {
	suite.Run(t, new(DistroByIdSuite))
}

func (s *DistroByIdSuite) SetupSuite() {
	s.data = data.MockDistroConnector{
		CachedDistros: []*distro.Distro{
			{Id: "distro1"},
			{Id: "distro2"},
		},
		CachedTasks: []task.Task{
			{Id: "task1"},
			{Id: "task2"},
		},
	}
	s.sc = &data.MockConnector{
		MockDistroConnector: s.data,
	}
}

func (s *DistroByIdSuite) SetupTest() {
	s.rm = makeGetDistroByID(s.sc)
}

func (s *DistroByIdSuite) TestFindByIdFound() {
	s.rm.(*distroIDGetHandler).distroId = "distro1"
	resp := s.rm.Run(context.TODO())
	s.NotNil(resp)
	s.Equal(resp.Status(), http.StatusOK)
	s.NotNil(resp.Data())

	d, ok := (resp.Data()).(*model.APIDistro)
	s.True(ok)
	s.Equal(model.ToAPIString("distro1"), d.Name)
}

func (s *DistroByIdSuite) TestFindByIdFail() {
	s.rm.(*distroIDGetHandler).distroId = "distro3"
	resp := s.rm.Run(context.TODO())
	s.NotNil(resp)
	s.NotEqual(resp.Status(), http.StatusOK)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH distro by id

type DistroPatchByIdSuite struct {
	sc   *data.MockConnector
	data data.MockDistroConnector
	rm   gimlet.RouteHandler

	suite.Suite
}

func TestDistroPatchSuite(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	suite.Run(t, new(DistroPatchByIdSuite))
}

func (s *DistroPatchByIdSuite) SetupSuite() {
	s.data = data.MockDistroConnector{
		CachedDistros: []*distro.Distro{
			{
				Id:       "fedora8",
				Arch:     "linux_amd64",
				WorkDir:  "/data/mci",
				PoolSize: 30,
				Provider: "mock",
				ProviderSettings: &map[string]interface{}{
					"bid_price":      0.133,
					"instance_type":  "m3.large",
					"key_name":       "mci",
					"security_group": "mci",
					"ami":            "ami-2814683f",
				},
				SetupAsSudo:  true,
				Setup:        "Set-up string",
				Teardown:     "Tear-down string",
				User:         "root",
				SSHKey:       "SSH key string",
				SSHOptions:   []string{"StrictHostKeyChecking=no", "BatchMode=yes", "ConnectTimeout=10"},
				SpawnAllowed: false,
				Expansions: []distro.Expansion{
					distro.Expansion{
						Key:   "decompress",
						Value: "tar zxvf"},
					distro.Expansion{
						Key:   "ps",
						Value: "ps aux"},
					distro.Expansion{
						Key:   "kill_pid",
						Value: "kill -- -$(ps opgid= %v)"},
					distro.Expansion{
						Key:   "scons_prune_ratio",
						Value: "0.8"},
				},
				Disabled:      false,
				ContainerPool: "",
			},
		},
	}
	s.sc = &data.MockConnector{
		MockDistroConnector: s.data,
	}
	s.rm = makePatchDistroByID(s.sc)
}

func (s *DistroPatchByIdSuite) TestParse() {
	ctx := context.Background()

	json := []byte(`{"ssh_options":["StrictHostKeyChecking=no","BatchMode=yes","ConnectTimeout=10"]}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/distros/fedora8", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)

	s.Equal(json, s.rm.(*distroIDPatchHandler).body)
}

func (s *DistroPatchByIdSuite) TestRunValidDistro() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})
	json := []byte(`{"user": "user1"}`)

	s.rm.(*distroIDPatchHandler).distroId = "fedora8"
	s.rm.(*distroIDPatchHandler).body = json
	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
}
