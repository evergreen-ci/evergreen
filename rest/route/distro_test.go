package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/distros/{distro_id}/setup

type DistroSetupByIDSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroSetupByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroSetupByIDSuite))
}

func (s *DistroSetupByIDSuite) SetupSuite() {
	s.sc = getMockDistrosConnector()
	s.rm = makeGetDistroSetup(s.sc)
}

func (s *DistroSetupByIDSuite) TestRunValidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDGetSetupHandler)
	h.distroID = "fedora8"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	script := resp.Data()
	s.Equal(script, model.ToAPIString("Set-up script"))
}

func (s *DistroSetupByIDSuite) TestRunInvalidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDGetSetupHandler)
	h.distroID = "invalid"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/distros/{distro_id}/setup

type DistroPatchSetupByIDSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroPatchSetupByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroPatchSetupByIDSuite))
}

func (s *DistroPatchSetupByIDSuite) SetupSuite() {
	s.sc = getMockDistrosConnector()
	s.rm = makeChangeDistroSetup(s.sc)
}

func (s *DistroPatchSetupByIDSuite) TestParseValidJSON() {
	ctx := context.Background()
	json := []byte(`{"setup": "New set-up script"}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/distros/fedora8/setup", bytes.NewBuffer(json))

	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.Equal("New set-up script", s.rm.(*distroIDChangeSetupHandler).Setup)
}

func (s *DistroPatchSetupByIDSuite) TestParseInvalidJSON() {
	ctx := context.Background()
	json := []byte(`{"malform": "ed}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/distros/fedora8/setup", bytes.NewBuffer(json))

	err := s.rm.Parse(ctx, req)
	s.Error(err)
}

func (s *DistroPatchSetupByIDSuite) TestRunValidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDChangeSetupHandler)
	h.distroID = "fedora8"
	h.Setup = "New set-up script"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Setup, model.ToAPIString("New set-up script"))
}

func (s *DistroPatchSetupByIDSuite) TestRunInvalidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDChangeSetupHandler)
	h.distroID = "invalid"
	h.Setup = "New set-up script"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/distros/{distro_id}/teardown

type DistroTeardownByIDSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroTeardownByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroTeardownByIDSuite))
}

func (s *DistroTeardownByIDSuite) SetupSuite() {
	s.sc = getMockDistrosConnector()
	s.rm = makeGetDistroTeardown(s.sc)
}

func (s *DistroTeardownByIDSuite) TestRunValidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDGetTeardownHandler)
	h.distroID = "fedora8"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	script := resp.Data()
	s.Equal(script, model.ToAPIString("Tear-down script"))
}

func (s *DistroTeardownByIDSuite) TestRunInvalidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDGetTeardownHandler)
	h.distroID = "invalid"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/distros/{distro_id}/teardown

type DistroPatchTeardownByIDSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroPatchTeardownByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroPatchTeardownByIDSuite))
}

func (s *DistroPatchTeardownByIDSuite) SetupSuite() {
	s.sc = getMockDistrosConnector()
	s.rm = makeChangeDistroTeardown(s.sc)
}

func (s *DistroPatchTeardownByIDSuite) TestParseValidJSON() {
	ctx := context.Background()
	json := []byte(`{"teardown": "New tear-down script"}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/distros/fedora8/teardown", bytes.NewBuffer(json))

	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.Equal("New tear-down script", s.rm.(*distroIDChangeTeardownHandler).Teardown)
}

func (s *DistroPatchTeardownByIDSuite) TestParseInvalidJSON() {
	ctx := context.Background()
	json := []byte(`{"malform": "ed}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/distros/fedora8/teardown", bytes.NewBuffer(json))

	err := s.rm.Parse(ctx, req)
	s.Error(err)
}

func (s *DistroPatchTeardownByIDSuite) TestRunValidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDChangeTeardownHandler)
	h.distroID = "fedora8"
	h.Teardown = "New tear-down script"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Teardown, model.ToAPIString("New tear-down script"))
}

func (s *DistroPatchTeardownByIDSuite) TestRunInvalidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDChangeTeardownHandler)
	h.distroID = "invalid"
	h.Teardown = "New set-up script"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/distros/{distro_id}

type DistroByIDSuite struct {
	sc   *data.MockConnector
	data data.MockDistroConnector
	rm   gimlet.RouteHandler

	suite.Suite
}

func TestDistroByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroByIDSuite))
}

func (s *DistroByIDSuite) SetupSuite() {
	s.data = data.MockDistroConnector{
		CachedDistros: []*distro.Distro{
			{
				Id: "distro1",
				PlannerSettings: distro.PlannerSettings{
					Version:                evergreen.PlannerVersionLegacy,
					MinimumHosts:           5,
					MaximumHosts:           10,
					TargetTime:             60000000000,
					AcceptableHostIdleTime: 10000000000,
					GroupVersions:          true,
					PatchZipperFactor:      7,
					MainlineFirst:          false,
					PatchFirst:             true,
				},
				BootstrapMethod:     distro.BootstrapMethodLegacySSH,
				CommunicationMethod: distro.CommunicationMethodLegacySSH,
				CloneMethod:         distro.CloneMethodLegacySSH,
			},
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

func (s *DistroByIDSuite) SetupTest() {
	s.rm = makeGetDistroByID(s.sc)
}

func (s *DistroByIDSuite) TestFindByIdFound() {
	s.rm.(*distroIDGetHandler).distroID = "distro1"

	resp := s.rm.Run(context.TODO())
	s.NotNil(resp)
	s.Equal(resp.Status(), http.StatusOK)
	s.NotNil(resp.Data())

	d, ok := (resp.Data()).(*model.APIDistro)
	s.True(ok)
	s.Equal(model.ToAPIString("distro1"), d.Name)
	s.Equal(model.ToAPIString(evergreen.PlannerVersionLegacy), d.PlannerSettings.Version)
	s.Equal(5, d.PlannerSettings.MinimumHosts)
	s.Equal(10, d.PlannerSettings.MaximumHosts)
	s.Equal(model.NewAPIDuration(60000000000), d.PlannerSettings.TargetTime)
	s.Equal(model.NewAPIDuration(10000000000), d.PlannerSettings.AcceptableHostIdleTime)
	s.Equal(true, d.PlannerSettings.GroupVersions)
	s.Equal(7, d.PlannerSettings.PatchZipperFactor)
	s.Equal(false, d.PlannerSettings.MainlineFirst)
	s.Equal(true, d.PlannerSettings.PatchFirst)
	s.Equal(model.ToAPIString(distro.BootstrapMethodLegacySSH), d.BootstrapMethod)
	s.Equal(model.ToAPIString(distro.CommunicationMethodLegacySSH), d.CommunicationMethod)
	s.Equal(model.ToAPIString(distro.CloneMethodLegacySSH), d.CloneMethod)
	s.Equal(model.ToAPIString(evergreen.FinderVersionLegacy), d.FinderSettings.Version)
}

func (s *DistroByIDSuite) TestFindByIdFail() {
	s.rm.(*distroIDGetHandler).distroID = "distro3"

	resp := s.rm.Run(context.TODO())
	s.NotNil(resp)
	s.NotEqual(resp.Status(), http.StatusOK)
}

///////////////////////////////////////////////////////////////////////
//
// Tests for PUT /rest/v2/distro/{distro_id}

type DistroPutSuite struct {
	sc       *data.MockConnector
	data     data.MockDistroConnector
	rm       gimlet.RouteHandler
	settings *evergreen.Settings

	suite.Suite
}

func TestDistroPutSuite(t *testing.T) {
	suite.Run(t, new(DistroPutSuite))
}

func (s *DistroPutSuite) SetupTest() {
	s.data = data.MockDistroConnector{
		CachedDistros: []*distro.Distro{
			{
				Id: "distro1",
			},
			{
				Id: "distro2",
			},
			{
				Id: "distro3",
			},
		},
	}
	s.sc = &data.MockConnector{
		MockDistroConnector: s.data,
	}
	s.settings = &evergreen.Settings{}
	s.rm = makePutDistro(s.sc, s.settings)
}

func (s *DistroPutSuite) TestParse() {
	ctx := context.Background()
	json := []byte(`
  	{
		"arch": "linux_amd64",
    	"work_dir": "/data/mci",
    	"ssh_key": "SSH string",
    	"provider": "mock",
    	"user": "tibor",
    	"planner_settings": {
      	"version": "tunable",
    		"minimum_hosts": 10,
    		"maximum_hosts": 20,
    		"target_time": 30000000000,
    		"acceptable_host_idle_time": 5000000000,
    		"group_versions": false,
    		"patch_zipper_factor": 2,
    		"mainline_first": true,
    		"patch_first": false
  		},
		"bootstrap_method": "legacy-ssh",
		"communication_method": "legacy-ssh",
		"clone_method": "legacy-ssh",
    }`,
	)

	req, _ := http.NewRequest("PUT", "http://example.com/api/rest/v2/distros/distro4", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
}

func (s *DistroPutSuite) TestRunNewWithValidEntity() {
	ctx := context.Background()
	json := []byte(`{"arch": "linux_amd64", "work_dir": "/data/mci", "ssh_key": "SSH Key", "provider": "mock", "user": "tibor"}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro4"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusCreated, resp.Status())
}

func (s *DistroPutSuite) TestRunNewWithInvalidEntity() {
	ctx := context.Background()
	json := []byte(`
	{
		"arch": "linux_amd64",
		"work_dir": "/data/mci",
		"ssh_key": "",
		"bootstrap_method": "foo",
		"communication_method": "bar",
		"clone_method": "bat",
		"provider": "mock",
		"user": "tibor",
		"planner_settings": {"version": "invalid"}
	}
	`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro4"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusBadRequest)
	err := (resp.Data()).(gimlet.ErrorResponse)
	s.Contains(err.Message, "ERROR: distro 'ssh_key' cannot be blank")
	s.Contains(err.Message, "'foo' is not a valid bootstrap method")
	s.Contains(err.Message, "'bar' is not a valid communication method")
	s.Contains(err.Message, "'bat' is not a valid clone method")
	s.Contains(err.Message, "ERROR: invalid PlannerSettings.Version 'invalid' for distro 'distro4'")
}

func (s *DistroPutSuite) TestRunNewConflictingName() {
	ctx := context.Background()
	json := []byte(`{"name": "distro5", "arch": "linux_amd64", "work_dir": "/data/mci", "ssh_key": "", "provider": "mock", "user": "tibor"}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro4"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusForbidden)
	error := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(error.Message, fmt.Sprintf("A distro's name is immutable; cannot rename distro '%s'", h.distroID))
}

func (s *DistroPutSuite) TestRunExistingWithValidEntity() {
	ctx := context.Background()
	json := []byte(`{"arch": "linux_amd64", "work_dir": "/data/mci", "ssh_key": "SSH Key", "provider": "mock", "user": "tibor"}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro3"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())
}

func (s *DistroPutSuite) TestRunExistingWithInvalidEntity() {
	ctx := context.Background()
	json := []byte(`{"arch": "", "work_dir": "/data/mci", "ssh_key": "SSH Key", "provider": "", "user": ""}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro3"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusBadRequest)
	err := (resp.Data()).(gimlet.ErrorResponse)
	s.Contains(err.Message, "ERROR: distro 'arch' cannot be blank")
	s.Contains(err.Message, "ERROR: distro 'user' cannot be blank")
	s.Contains(err.Message, "ERROR: distro 'provider' cannot be blank")
}

func (s *DistroPutSuite) TestRunExistingConflictingName() {
	ctx := context.Background()
	json := []byte(`{"name": "distro5", "arch": "linux_amd64", "work_dir": "/data/mci", "ssh_key": "", "provider": "mock", "user": "tibor"}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro3"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusForbidden)
	error := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(fmt.Sprintf("A distro's name is immutable; cannot rename distro '%s'", h.distroID), error.Message)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for DELETE /rest/v2/distros/{distro_id}

type DistroDeleteByIDSuite struct {
	sc   *data.MockConnector
	data data.MockDistroConnector
	rm   gimlet.RouteHandler

	suite.Suite
}

func TestDistroDeleteSuite(t *testing.T) {
	suite.Run(t, new(DistroDeleteByIDSuite))
}

func (s *DistroDeleteByIDSuite) SetupTest() {
	s.data = data.MockDistroConnector{
		CachedDistros: []*distro.Distro{
			{
				Id: "distro1",
			},
			{
				Id: "distro2",
			},
			{
				Id: "distro3",
			},
		},
	}
	s.sc = &data.MockConnector{
		MockDistroConnector: s.data,
	}
	s.rm = makeDeleteDistroByID(s.sc)
}

func (s *DistroDeleteByIDSuite) TestParse() {
	ctx := context.Background()

	req, _ := http.NewRequest("DELETE", "http://example.com/api/rest/v2/distros/distro1", nil)
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
}

func (s *DistroDeleteByIDSuite) TestRunValidDistroId() {
	ctx := context.Background()
	h := s.rm.(*distroIDDeleteHandler)
	h.distroID = "distro1"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
}

func (s *DistroDeleteByIDSuite) TestRunInvalidDistroId() {
	ctx := context.Background()
	h := s.rm.(*distroIDDeleteHandler)
	h.distroID = "distro4"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
	error := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(error.Message, fmt.Sprintf("distro with id '%s' not found", h.distroID))
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/distros/{distro_id}

type DistroPatchByIDSuite struct {
	sc       *data.MockConnector
	data     data.MockDistroConnector
	rm       gimlet.RouteHandler
	settings *evergreen.Settings

	suite.Suite
}

func TestDistroPatchSuite(t *testing.T) {
	suite.Run(t, new(DistroPatchByIDSuite))
}

func (s *DistroPatchByIDSuite) SetupTest() {
	s.data = data.MockDistroConnector{
		CachedDistros: []*distro.Distro{
			{
				Id:       "fedora8",
				Arch:     "linux_amd64",
				WorkDir:  "/data/mci",
				PoolSize: 30,
				Provider: "mock",
				ProviderSettings: &map[string]interface{}{
					"bid_price":      0.2,
					"instance_type":  "m3.large",
					"key_name":       "mci",
					"security_group": "mci",
					"ami":            "ami-2814683f",
					"mount_points": map[string]interface{}{
						"device_name":  "/dev/xvdb",
						"virtual_name": "ephemeral0"},
				},
				SetupAsSudo: true,
				Setup:       "Set-up string",
				Teardown:    "Tear-down string",
				User:        "root",
				SSHKey:      "SSH key string",
				SSHOptions: []string{
					"StrictHostKeyChecking=no",
					"BatchMode=yes",
					"ConnectTimeout=10"},
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
	s.settings = &evergreen.Settings{}
	s.sc = &data.MockConnector{
		MockDistroConnector: s.data,
	}
	s.rm = makePatchDistroByID(s.sc, s.settings)
}

func (s *DistroPatchByIDSuite) TestParse() {
	ctx := context.Background()
	json := []byte(`{"ssh_options":["StrictHostKeyChecking=no","BatchMode=yes","ConnectTimeout=10"]}`)
	req, _ := http.NewRequest("PATCH", "http://example.com/api/rest/v2/distros/fedora8", bytes.NewBuffer(json))

	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.Equal(json, s.rm.(*distroIDPatchHandler).body)
}

func (s *DistroPatchByIDSuite) TestRunValidSpawnAllowed() {
	ctx := context.Background()
	json := []byte(`{"user_spawn_allowed": true}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.UserSpawnAllowed, true)
}

func (s *DistroPatchByIDSuite) TestRunValidProvider() {
	ctx := context.Background()
	json := []byte(`{"provider": "mock"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Provider, model.ToAPIString("mock"))
}

func (s *DistroPatchByIDSuite) TestRunValidProviderSettings() {
	ctx := context.Background()
	json := []byte(
		`{"settings" :{"bid_price": 0.15, "security_group": "password123"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	points := apiDistro.ProviderSettings["mount_points"]
	mapped := points.(map[string]interface{})
	s.Equal(mapped["device_name"], "/dev/xvdb")
	s.Equal(mapped["virtual_name"], "ephemeral0")
	s.Equal(apiDistro.ProviderSettings["bid_price"], 0.15)
	s.Equal(apiDistro.ProviderSettings["instance_type"], "m3.large")
	s.Equal(apiDistro.ProviderSettings["key_name"], "mci")
	s.Equal(apiDistro.ProviderSettings["security_group"], "password123")
	s.Equal(apiDistro.ProviderSettings["ami"], "ami-2814683f")
}

func (s *DistroPatchByIDSuite) TestRunValidArch() {
	ctx := context.Background()
	json := []byte(`{"arch": "linux_amd64"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Arch, model.ToAPIString("linux_amd64"))
}

func (s *DistroPatchByIDSuite) TestRunValidWorkDir() {
	ctx := context.Background()
	json := []byte(`{"work_dir": "/tmp"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.WorkDir, model.ToAPIString("/tmp"))
}

func (s *DistroPatchByIDSuite) TestRunValidPoolSize() {
	ctx := context.Background()
	json := []byte(`{"pool_size": 50}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.PoolSize, 50)
}

func (s *DistroPatchByIDSuite) TestRunValidSetupAsSudo() {
	ctx := context.Background()
	json := []byte(`{"setup_as_sudo": false}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.SetupAsSudo, false)
}

func (s *DistroPatchByIDSuite) TestRunValidSetup() {
	ctx := context.Background()
	json := []byte(`{"setup": "New Set-up string"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Setup, model.ToAPIString("New Set-up string"))
}

func (s *DistroPatchByIDSuite) TestRunValidTearDown() {
	ctx := context.Background()
	json := []byte(`{"teardown": "New Tear-down string"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Teardown, model.ToAPIString("New Tear-down string"))
}

func (s *DistroPatchByIDSuite) TestRunValidUser() {
	ctx := context.Background()
	json := []byte(`{"user": "user101"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.User, model.ToAPIString("user101"))
}

func (s *DistroPatchByIDSuite) TestRunValidSSHKey() {
	ctx := context.Background()
	json := []byte(`{"ssh_key": "New SSH key string"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.SSHKey, model.ToAPIString("New SSH key string"))
}

func (s *DistroPatchByIDSuite) TestRunValidSSHOptions() {
	ctx := context.Background()
	json := []byte(`{"ssh_options":["BatchMode=no"]}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.SSHOptions, []string{"BatchMode=no"})
}

func (s *DistroPatchByIDSuite) TestRunValidExpansions() {
	ctx := context.Background()
	json := []byte(`{"expansions": [{"key": "key1", "value": "value1"}]}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	expansion := model.APIExpansion{Key: model.ToAPIString("key1"), Value: model.ToAPIString("value1")}
	s.Equal(apiDistro.Expansions, []model.APIExpansion{expansion})
}

func (s *DistroPatchByIDSuite) TestRunValidDisabled() {
	ctx := context.Background()
	json := []byte(`{"disabled": true}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Disabled, true)
}

func (s *DistroPatchByIDSuite) TestRunValidContainer() {
	ctx := context.Background()
	json := []byte(`{"container_pool": ""}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.ContainerPool, model.ToAPIString(""))
	s.Equal(apiDistro.PlannerSettings.Version, model.ToAPIString("legacy"))
}

func (s *DistroPatchByIDSuite) TestRunInvalidEmptyStringValues() {
	ctx := context.Background()
	json := []byte(`{"arch": "","user": "","work_dir": "","ssh_key": "","provider": ""}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusBadRequest)
	s.NotNil(resp.Data())

	errors := []string{
		"ERROR: distro 'arch' cannot be blank",
		"ERROR: distro 'user' cannot be blank",
		"ERROR: distro 'work_dir' cannot be blank",
		"ERROR: distro 'ssh_key' cannot be blank",
		"ERROR: distro 'provider' cannot be blank",
	}

	error := (resp.Data()).(gimlet.ErrorResponse)
	for _, err := range errors {
		s.Contains(error.Message, err)
	}
}

func (s *DistroPatchByIDSuite) TestRunValidPlannerSettingsVersion() {
	ctx := context.Background()
	json := []byte(`{"planner_settings": {"version": "tunable"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(model.ToAPIString("tunable"), apiDistro.PlannerSettings.Version)
}

func (s *DistroPatchByIDSuite) TestRunInvalidPlannerSettingsVersion() {
	ctx := context.Background()
	json := []byte(`{"planner_settings": {"version": "invalid"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunInvalidFinderSettingsVersion() {
	ctx := context.Background()
	json := []byte(`{"finder_settings": {"version": "invalid"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunValidFinderSettingsVersion() {
	ctx := context.Background()
	json := []byte(`{"finder_settings": {"version": "legacy"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(model.ToAPIString("legacy"), apiDistro.PlannerSettings.Version)
}

func (s *DistroPatchByIDSuite) TestRunValidBootstrapMethod() {
	ctx := context.Background()
	json := []byte(`{"bootstrap_method": "legacy-ssh"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(model.ToAPIString(distro.BootstrapMethodLegacySSH), apiDistro.BootstrapMethod)
}

func (s *DistroPatchByIDSuite) TestRunInvalidBootstrapMethod() {
	ctx := context.Background()
	json := []byte(`{"bootstrap_method": "foobar"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunValidCommunicationMethod() {
	ctx := context.Background()
	json := []byte(`{"communication_method": "legacy-ssh"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(model.ToAPIString(distro.CommunicationMethodLegacySSH), apiDistro.CommunicationMethod)
}

func (s *DistroPatchByIDSuite) TestRunInvalidCommunicationMethod() {
	ctx := context.Background()
	json := []byte(`{"communication_method": "foobar"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunValidBootstrapAndCommunicationMethods() {
	ctx := context.Background()
	json := []byte(fmt.Sprintf(
		`{"bootstrap_method": "%s", "communication_method": "%s"} `,
		distro.BootstrapMethodLegacySSH, distro.CommunicationMethodLegacySSH))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(model.ToAPIString(distro.BootstrapMethodLegacySSH), apiDistro.BootstrapMethod)
	s.Equal(model.ToAPIString(distro.CommunicationMethodLegacySSH), apiDistro.CommunicationMethod)
}

func (s *DistroPatchByIDSuite) TestRunInvalidBootstrapAndCommunicationMethods() {
	ctx := context.Background()
	json := []byte(fmt.Sprintf(
		`{"bootstrap_method": "%s", "communication_method": "%s"} `,
		distro.BootstrapMethodUserData, distro.CommunicationMethodLegacySSH))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunValidCloneMethod() {
	ctx := context.Background()
	json := []byte(fmt.Sprintf(`{"clone_method": "%s"}`, distro.CloneMethodLegacySSH))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(model.ToAPIString(distro.CloneMethodLegacySSH), apiDistro.CloneMethod)
}

func (s *DistroPatchByIDSuite) TestRunInvalidCloneMethod() {
	ctx := context.Background()
	json := []byte(`{"clone_method": "foobar"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestValidFindAndReplaceFullDocument() {
	ctx := context.Background()
	json := []byte(
		`{
				"arch" : "linux_amd64",
				"work_dir" : "~/data/mci",
				"pool_size" : 20,
				"provider" : "mock",
				"settings" : {
					"mount_points" : [{
						"device_name" : "~/dev/xvdb",
						"virtual_name" : "~ephemeral0"
					}],
					"ami" : "~ami-2814683f",
					"bid_price" : 0.1,
					"instance_type" : "~m3.large",
					"key_name" : "~mci",
					"security_group" : "~mci"
				},
				"setup_as_sudo" : false,
				"setup" : "~Set-up script",
				"teardown" : "~Tear-down script",
				"user" : "~root",
				"bootstrap_method": "legacy-ssh",
				"communication_method": "legacy-ssh",
				"clone_method": "legacy-ssh",
				"shell_path": "/usr/bin/bash",
				"curator_dir": "/usr/local/bin",
				"ssh_key" : "~SSH string",
				"ssh_options" : [
					"~StrictHostKeyChecking=no",
					"~BatchMode=no",
					"~ConnectTimeout=10"
				],
				"spawn_allowed" : false,
				"expansions" : [
					{
						"key" : "~decompress",
						"value" : "~tar zxvf"
					},
					{
						"key" : "~ps",
						"value" : "~ps aux"
					},
					{
						"key" : "~kill_pid",
						"value" : "~kill -- -$(ps opgid= %v)"
					},
					{
						"key" : "~scons_prune_ratio",
						"value" : "~0.8"
					}
				],
				"disabled" : false
	}`)

	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.Equal(resp.Status(), http.StatusOK)

	s.NotNil(resp.Data())
	apiDistro, ok := (resp.Data()).(*model.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Disabled, false)
	s.Equal(apiDistro.Name, model.ToAPIString("fedora8"))
	s.Equal(apiDistro.WorkDir, model.ToAPIString("~/data/mci"))
	s.Equal(apiDistro.PoolSize, 20)
	s.Equal(apiDistro.Provider, model.ToAPIString("mock"))

	points := apiDistro.ProviderSettings["mount_points"]
	typed := points.([]interface{})
	mapped := typed[0].(map[string]interface{})
	s.Equal(mapped["device_name"], "~/dev/xvdb")
	s.Equal(mapped["virtual_name"], "~ephemeral0")

	s.Equal(apiDistro.ProviderSettings["ami"], "~ami-2814683f")
	s.Equal(apiDistro.ProviderSettings["bid_price"], 0.1)
	s.Equal(apiDistro.ProviderSettings["instance_type"], "~m3.large")
	s.Equal(apiDistro.SetupAsSudo, false)
	s.Equal(apiDistro.Setup, model.ToAPIString("~Set-up script"))
	s.Equal(apiDistro.Teardown, model.ToAPIString("~Tear-down script"))
	s.Equal(model.ToAPIString(distro.BootstrapMethodLegacySSH), apiDistro.BootstrapMethod)
	s.Equal(model.ToAPIString(distro.CommunicationMethodLegacySSH), apiDistro.CommunicationMethod)
	s.Equal(model.ToAPIString(distro.CloneMethodLegacySSH), apiDistro.CloneMethod)
	s.Equal(model.ToAPIString("/usr/bin/bash"), apiDistro.ShellPath)
	s.Equal(model.ToAPIString("/usr/local/bin", apiDistro.CuratorDir))
	s.Equal(apiDistro.User, model.ToAPIString("~root"))
	s.Equal(apiDistro.SSHKey, model.ToAPIString("~SSH string"))
	s.Equal(apiDistro.SSHOptions, []string{"~StrictHostKeyChecking=no", "~BatchMode=no", "~ConnectTimeout=10"})
	s.Equal(apiDistro.UserSpawnAllowed, false)

	s.Equal(apiDistro.Expansions, []model.APIExpansion{
		model.APIExpansion{Key: model.ToAPIString("~decompress"), Value: model.ToAPIString("~tar zxvf")},
		model.APIExpansion{Key: model.ToAPIString("~ps"), Value: model.ToAPIString("~ps aux")},
		model.APIExpansion{Key: model.ToAPIString("~kill_pid"), Value: model.ToAPIString("~kill -- -$(ps opgid= %v)")},
		model.APIExpansion{Key: model.ToAPIString("~scons_prune_ratio"), Value: model.ToAPIString("~0.8")},
	})
}

func (s *DistroPatchByIDSuite) TestRunInvalidNameChange() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})
	json := []byte(`{"name": "Updated distro name"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusForbidden)

	gimlet := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(gimlet.Message, fmt.Sprintf("A distro's name is immutable; cannot rename distro '%s'", h.distroID))
}

func getMockDistrosConnector() *data.MockConnector {
	connector := data.MockConnector{
		MockDistroConnector: data.MockDistroConnector{
			CachedDistros: []*distro.Distro{
				{
					Id:       "fedora8",
					Arch:     "linux_amd64",
					WorkDir:  "/data/mci",
					PoolSize: 30,
					Provider: "mock",
					ProviderSettings: &map[string]interface{}{
						"bid_price":      0.2,
						"instance_type":  "m3.large",
						"key_name":       "mci",
						"security_group": "mci",
						"ami":            "ami-2814683f",
						"mount_points": map[string]interface{}{
							"device_name":  "/dev/xvdb",
							"virtual_name": "ephemeral0"},
					},
					SetupAsSudo: true,
					Setup:       "Set-up script",
					Teardown:    "Tear-down script",
					User:        "root",
					SSHKey:      "SSH key string",
					SSHOptions: []string{
						"StrictHostKeyChecking=no",
						"BatchMode=yes",
						"ConnectTimeout=10"},
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
		},
	}

	return &connector
}
