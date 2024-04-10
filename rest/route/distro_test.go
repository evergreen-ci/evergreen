package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/distros/{distro_id}/setup

type DistroSetupByIDSuite struct {
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroSetupByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroSetupByIDSuite))
}

func (s *DistroSetupByIDSuite) SetupSuite() {
	s.NoError(db.ClearCollections(distro.Collection))
	s.NoError(getMockDistrosdata())
	s.rm = makeGetDistroSetup()
}

func (s *DistroSetupByIDSuite) TestRunValidId() {
	ctx := context.Background()
	h := s.rm.(*distroIDGetSetupHandler)
	h.distroID = "fedora8"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	script := resp.Data()
	s.Equal(script, utility.ToStringPtr("Set-up script"))
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
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroPatchSetupByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroPatchSetupByIDSuite))
}

func (s *DistroPatchSetupByIDSuite) SetupSuite() {
	s.NoError(db.ClearCollections(distro.Collection))
	s.NoError(getMockDistrosdata())
	s.rm = makeChangeDistroSetup()
}

func (s *DistroPatchSetupByIDSuite) TestParseValidJSON() {
	ctx := context.Background()
	json := []byte(`{"setup": "New set-up script"}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/distros/fedora8/setup", bytes.NewBuffer(json))

	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.Equal("New set-up script", s.rm.(*distroIDChangeSetupHandler).Setup)
}

func (s *DistroPatchSetupByIDSuite) TestParseInvalidJSON() {
	ctx := context.Background()
	json := []byte(`{"malform": "ed}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/distros/fedora8/setup", bytes.NewBuffer(json))

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

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Setup, utility.ToStringPtr("New set-up script"))
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
// Tests for GET /rest/v2/distros/{distro_id}

type DistroByIDSuite struct {
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroByIDSuite(t *testing.T) {
	suite.Run(t, new(DistroByIDSuite))
}

func (s *DistroByIDSuite) SetupSuite() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(distro.Collection))
	distros := []*distro.Distro{
		{
			Id: "distro1",
			DispatcherSettings: distro.DispatcherSettings{
				Version: evergreen.DispatcherVersionRevisedWithDependencies,
			},
			HostAllocatorSettings: distro.HostAllocatorSettings{
				Version:                evergreen.HostAllocatorUtilization,
				MinimumHosts:           5,
				MaximumHosts:           10,
				AcceptableHostIdleTime: 10000000000,
			},
			FinderSettings: distro.FinderSettings{
				Version: evergreen.FinderVersionLegacy,
			},
			PlannerSettings: distro.PlannerSettings{
				Version:       evergreen.PlannerVersionTunable,
				TargetTime:    80000000000,
				GroupVersions: utility.TruePtr(),
				PatchFactor:   7,
			},
			BootstrapSettings: distro.BootstrapSettings{
				Method:        distro.BootstrapMethodLegacySSH,
				Communication: distro.CommunicationMethodLegacySSH,
			},
			CloneMethod: evergreen.CloneMethodLegacySSH,
		},
		{Id: "distro2"},
	}

	for _, d := range distros {
		err := d.Insert(ctx)
		s.NoError(err)
	}
}

func (s *DistroByIDSuite) SetupTest() {
	s.rm = makeGetDistroByID()
}

func (s *DistroByIDSuite) TestFindByIdFound() {
	s.rm.(*distroIDGetHandler).distroID = "distro1"

	resp := s.rm.Run(context.TODO())
	s.NotNil(resp)
	s.Equal(resp.Status(), http.StatusOK)
	s.NotNil(resp.Data())

	d, ok := (resp.Data()).(*restModel.APIDistro)

	s.True(ok)
	s.Equal(utility.ToStringPtr("distro1"), d.Name)

	s.Equal(5, d.HostAllocatorSettings.MinimumHosts)
	s.Equal(10, d.HostAllocatorSettings.MaximumHosts)
	s.Equal(restModel.NewAPIDuration(10000000000), d.HostAllocatorSettings.AcceptableHostIdleTime)
	s.Equal(utility.ToStringPtr(evergreen.PlannerVersionTunable), d.PlannerSettings.Version)
	s.Equal(restModel.NewAPIDuration(80000000000), d.PlannerSettings.TargetTime)
	s.Equal(true, d.PlannerSettings.GroupVersions)
	s.EqualValues(7, d.PlannerSettings.PatchFactor)
	s.Equal(utility.ToStringPtr(distro.BootstrapMethodLegacySSH), d.BootstrapSettings.Method)
	s.Equal(utility.ToStringPtr(distro.CommunicationMethodLegacySSH), d.BootstrapSettings.Communication)
	s.Equal(utility.ToStringPtr(evergreen.CloneMethodLegacySSH), d.CloneMethod)
	s.Equal(utility.ToStringPtr(evergreen.FinderVersionLegacy), d.FinderSettings.Version)
	s.Equal(utility.ToStringPtr(evergreen.DispatcherVersionRevisedWithDependencies), d.DispatcherSettings.Version)
}

func (s *DistroByIDSuite) TestFindByIdFail() {
	s.rm.(*distroIDGetHandler).distroID = "distro3"

	resp := s.rm.Run(context.TODO())
	s.NotNil(resp)
	s.NotEqual(resp.Status(), http.StatusOK)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/distros/{distro_id}/ami

func TestDistroAMIHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(distro.Collection))
	d := distro.Distro{
		Id:       "d1",
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{
			birch.NewDocument(
				birch.EC.String("ami", "ami-1234"),
				birch.EC.String("region", "us-east-1"),
			),
			birch.NewDocument(
				birch.EC.String("ami", "ami-5678"),
				birch.EC.String("region", "us-west-1"),
			),
		},
	}
	assert.NoError(t, d.Insert(ctx))
	h := makeGetDistroAMI().(*distroAMIHandler)

	// default region
	r, err := http.NewRequest(http.MethodGet, "/distros/d1/ami", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"distro_id": "d1"})
	assert.NoError(t, h.Parse(context.TODO(), r))
	assert.Equal(t, "d1", h.distroID)

	resp := h.Run(context.TODO())
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	ami, ok := resp.Data().(string)
	assert.True(t, ok)
	assert.Equal(t, "ami-1234", ami)

	// provided region
	r, err = http.NewRequest(http.MethodGet, "/distros/d1/ami?region=us-west-1", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"distro_id": "d1"})
	assert.NoError(t, h.Parse(context.TODO(), r))
	assert.Equal(t, "d1", h.distroID)
	assert.Equal(t, "us-west-1", h.region)

	resp = h.Run(context.TODO())
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
	ami, ok = resp.Data().(string)
	assert.True(t, ok)
	assert.Equal(t, "ami-5678", ami)

	// fake region
	h.region = "fake"
	resp = h.Run(context.TODO())
	assert.NotNil(t, resp)
	assert.NotEqual(t, http.StatusOK, resp.Status())
}

///////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/distros/settings

func TestUpdateDistrosSettingsHandlerParse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(distro.Collection))
	d := distro.Distro{
		Id:       "d1",
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{
			birch.NewDocument(
				birch.EC.String("ami", "ami-1234"),
				birch.EC.String("region", "us-east-1"),
			),
			birch.NewDocument(
				birch.EC.String("ami", "ami-5678"),
				birch.EC.String("region", "us-west-1"),
			),
		},
	}
	assert.NoError(t, d.Insert(ctx))
	h := makeModifyDistrosSettings().(*modifyDistrosSettingsHandler)

	jsonChanges := `{"region": ""}`
	body := bytes.NewBuffer([]byte(jsonChanges))
	r, err := http.NewRequest(http.MethodPatch, "/distros/settings", body)
	assert.NoError(t, err)
	assert.Error(t, h.Parse(context.TODO(), r))

	h = makeModifyDistrosSettings().(*modifyDistrosSettingsHandler)
	jsonChanges = `{"region": "us-east-1", "ami": "ami-new"}`
	body = bytes.NewBuffer([]byte(jsonChanges))
	r, err = http.NewRequest(http.MethodPatch, "/distros/settings", body)
	assert.NoError(t, err)

	assert.NoError(t, h.Parse(context.TODO(), r))
	assert.Equal(t, "us-east-1", h.region)

	assert.NotNil(t, h.settings)
	assert.Equal(t, "ami-new", h.settings.Lookup("ami").StringValue())
}

func TestUpdateDistrosSettingsHandlerRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(distro.Collection, event.EventCollection))
	conf := testutil.TestConfig()
	conf.Providers.AWS.EC2Keys = []evergreen.EC2Key{{Key: "key", Secret: "secret"}}
	conf.SSHKeyPairs = []evergreen.SSHKeyPair{{Name: "a"}}
	assert.NoError(t, evergreen.UpdateConfig(ctx, conf))
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "userName"})

	d := &distro.Distro{Id: "d1", Arch: "linux_amd64", User: "a", SSHKey: "a", WorkDir: "a",
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{
			birch.NewDocument(
				birch.EC.String("region", "us-east-1"),
				birch.EC.String("ami", "ami-123"),
				birch.EC.String("key_name", "a"),
				birch.EC.String("instance_type", "a"),
				birch.EC.SliceString("security_group_ids", []string{"a"}),
			),
			birch.NewDocument(
				birch.EC.String("region", "us-west-1"),
				birch.EC.String("ami", "ami-234"),
				birch.EC.String("key_name", "a"),
				birch.EC.String("instance_type", "a"),
				birch.EC.SliceString("security_group_ids", []string{"a1", "b1"}),
			),
		},
		PlannerSettings: distro.PlannerSettings{
			Version: evergreen.PlannerVersionTunable,
		},
		BootstrapSettings: distro.BootstrapSettings{
			Method:        distro.BootstrapMethodLegacySSH,
			Communication: distro.CommunicationMethodLegacySSH,
		},
		CloneMethod: evergreen.CloneMethodLegacySSH,
		FinderSettings: distro.FinderSettings{
			Version: evergreen.FinderVersionLegacy,
		},
		DispatcherSettings: distro.DispatcherSettings{
			Version: evergreen.DispatcherVersionRevisedWithDependencies,
		},
		HostAllocatorSettings: distro.HostAllocatorSettings{
			Version:      evergreen.HostAllocatorUtilization,
			MinimumHosts: 10,
			MaximumHosts: 20,
		},
	}
	assert.NoError(t, d.Insert(ctx))
	h := makeModifyDistrosSettings().(*modifyDistrosSettingsHandler)

	h.settings = birch.NewDocument(
		birch.EC.String("region", "us-west-1"),
		birch.EC.SliceString("security_group_ids", []string{"c1", "d1"}),
		birch.EC.String("ami", "ami-456"),
	)
	h.region = "us-west-1"

	resp := h.Run(ctx)
	assert.Equal(t, http.StatusOK, resp.Status())

	distroFromDB, err := distro.FindOneId(ctx, "d1")
	assert.NoError(t, err)
	assert.NotNil(t, distroFromDB)
	assert.Len(t, distroFromDB.ProviderSettingsList, 2)

	for _, s := range distroFromDB.ProviderSettingsList {
		settings := cloud.EC2ProviderSettings{}
		assert.NoError(t, settings.FromDocument(s))
		if settings.Region == "us-east-1" {
			assert.Equal(t, "ami-123", settings.AMI)
		} else if settings.Region == "us-west-1" {
			assert.Equal(t, "ami-456", settings.AMI)
			assert.Equal(t, []string{"c1", "d1"}, settings.SecurityGroupIDs)
		}
	}

	events, err := event.FindAllByResourceID("d1")
	assert.NoError(t, err)
	// Doesn't include the AMI update because it's not a default region that was updated.
	assert.Len(t, events, 1)

}

///////////////////////////////////////////////////////////////////////
//
// Tests for PUT /rest/v2/distros/{distro_id}

type DistroPutSuite struct {
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroPutSuite(t *testing.T) {
	suite.Run(t, new(DistroPutSuite))
}

func (s *DistroPutSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(distro.Collection))
	distros := []*distro.Distro{
		{
			Id: "distro1",
		},
		{
			Id: "distro2",
		},
		{
			Id: "distro3",
		},
		{
			Id: "distro4",
		},
	}
	settings := &evergreen.Settings{
		SSHKeyPairs: []evergreen.SSHKeyPair{
			{
				Name:    "SSH Key",
				Public:  "public_key",
				Private: "private_key",
			},
		},
	}
	for _, d := range distros {
		err := d.Insert(ctx)
		s.NoError(err)
	}
	s.NoError(evergreen.UpdateConfig(ctx, settings))
	s.rm = makePutDistro()
}

func (s *DistroPutSuite) TestParse() {
	ctx := context.Background()
	json := []byte(`
	{
		"arch": "linux_amd64",
		"work_dir": "/data/mci",
		"ssh_key": "SSH key",
		"provider": "mock",
		"user": "tibor",
		"planner_settings": {
		"version": "tunable",
			"minimum_hosts": 10,
			"maximum_hosts": 20,
			"target_time": 30000000000,
			"acceptable_host_idle_time": 5000000000,
			"group_versions": false,
			"patch_factor": 2,
			"patch_first": false
		},
		"bootstrap_settings": {"method": "legacy-ssh", "communication": "legacy-ssh"},
		"clone_method": "legacy-ssh",
	}`,
	)

	req, _ := http.NewRequest(http.MethodPut, "http://example.com/api/rest/v2/distros/distro4", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
}

func (s *DistroPutSuite) TestRunNewWithValidEntity() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	json := []byte(`{"arch": "linux_amd64", "work_dir": "/data/mci", "ssh_key": "SSH Key", "provider": "mock", "user": "tibor"}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro5"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusCreated, resp.Status())
}

func (s *DistroPutSuite) TestRunNewWithInvalidEntity() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	json := []byte(`
	{
		"arch": "linux_amd64",
		"work_dir": "/data/mci",
		"ssh_key": "",
		"bootstrap_settings": {"method": "foo", "communication": "bar"},
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
	s.Contains(err.Message, "'foo' is not a valid bootstrap method")
	s.Contains(err.Message, "'bar' is not a valid communication method")
	s.Contains(err.Message, "ERROR: invalid planner_settings.version 'invalid' for distro 'distro4'")
}

func (s *DistroPutSuite) TestRunNewConflictingName() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	json := []byte(`{"name": "distro5", "arch": "linux_amd64", "work_dir": "/data/mci", "ssh_key": "SSH Key", "provider": "mock", "user": "tibor"}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro4"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusForbidden)
	err := resp.Data().(gimlet.ErrorResponse)
	s.Equal(fmt.Sprintf("distro name 'distro5' is immutable so it cannot be renamed to '%s'", h.distroID), err.Message)
}

func (s *DistroPutSuite) TestRunExistingWithValidEntity() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	json := []byte(`{"name": "distro5", "arch": "linux_amd64", "work_dir": "/data/mci", "ssh_key": "SSH Key", "provider": "mock", "user": "tibor"}`)
	h := s.rm.(*distroIDPutHandler)
	h.distroID = "distro3"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusForbidden)
	err := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(fmt.Sprintf("distro name 'distro5' is immutable so it cannot be renamed to '%s'", h.distroID), err.Message)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for DELETE /rest/v2/distros/{distro_id}

type DistroDeleteByIDSuite struct {
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroDeleteSuite(t *testing.T) {
	suite.Run(t, new(DistroDeleteByIDSuite))
}

func (s *DistroDeleteByIDSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(distro.Collection, model.TaskSecondaryQueuesCollection, model.TaskQueuesCollection))
	distros := []*distro.Distro{
		{
			Id: "distro1",
		},
		{
			Id: "distro2",
		},
		{
			Id: "distro3",
		},
	}
	for _, d := range distros {
		err := d.Insert(ctx)
		s.NoError(err)
	}
	s.rm = makeDeleteDistroByID()
}

func (s *DistroDeleteByIDSuite) TestParse() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})

	req, _ := http.NewRequest(http.MethodDelete, "http://example.com/api/rest/v2/distros/distro1", nil)
	err := s.rm.Parse(ctx, req)
	s.NoError(err)
}

func (s *DistroDeleteByIDSuite) TestRunValidDistroId() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})

	now := time.Now().Round(time.Millisecond).UTC()
	taskQueue := &model.TaskQueue{
		Distro:      "distro1",
		GeneratedAt: now,
	}
	s.NoError(db.Insert(model.TaskQueuesCollection, taskQueue))
	h := s.rm.(*distroIDDeleteHandler)
	h.distroID = "distro1"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
}

func (s *DistroDeleteByIDSuite) TestRunInvalidDistroId() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})

	h := s.rm.(*distroIDDeleteHandler)
	h.distroID = "distro4"

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusNotFound)
	err := (resp.Data()).(gimlet.ErrorResponse)
	s.Equal(fmt.Sprintf("distro '%s' not found", h.distroID), err.Message)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/distros/{distro_id}

type DistroPatchByIDSuite struct {
	rm gimlet.RouteHandler

	suite.Suite
}

func TestDistroPatchSuite(t *testing.T) {
	suite.Run(t, new(DistroPatchByIDSuite))
}

func (s *DistroPatchByIDSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sshKey := "SSH Key"
	settingsList := []*birch.Document{birch.NewDocument(
		birch.EC.Double("bid_price", 0.2),
		birch.EC.String("instance_type", "m3.large"),
		birch.EC.String("key_name", "mci"),
		birch.EC.String("security_group", "mci"),
		birch.EC.String("ami", "ami-2814683f"),
		birch.EC.Array("mount_points", birch.NewArray(
			birch.VC.Document(birch.NewDocument(
				birch.EC.String("device_name", "/dev/xvdb"),
				birch.EC.String("virtual_name", "ephemeral0"),
			)),
		)),
	)}
	s.NoError(db.ClearCollections(distro.Collection))
	distros := []*distro.Distro{
		{
			Id:      "fedora8",
			Arch:    evergreen.ArchLinuxAmd64,
			WorkDir: "/data/mci",
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MaximumHosts: 30,
			},
			Provider:             evergreen.ProviderNameMock,
			ProviderSettingsList: settingsList,
			SetupAsSudo:          true,
			Setup:                "Set-up string",
			User:                 "root",
			SSHKey:               sshKey,
			SSHOptions: []string{
				"StrictHostKeyChecking=no",
				"BatchMode=yes",
				"ConnectTimeout=10"},
			SpawnAllowed: false,
			Expansions: []distro.Expansion{
				{
					Key:   "decompress",
					Value: "tar zxvf"},
				{
					Key:   "ps",
					Value: "ps aux"},
				{
					Key:   "kill_pid",
					Value: "kill -- -$(ps opgid= %v)"},
				{
					Key:   "scons_prune_ratio",
					Value: "0.8"},
			},
			Disabled:      false,
			ContainerPool: "",
		},
	}
	settings := &evergreen.Settings{
		SSHKeyPairs: []evergreen.SSHKeyPair{
			{
				Name:    sshKey,
				Public:  "public",
				Private: "private",
			},
			{
				Name:    "New SSH Key",
				Public:  "new_public",
				Private: "new_private",
			},
		},
	}
	for _, d := range distros {
		err := d.Insert(ctx)
		s.NoError(err)
	}
	s.NoError(evergreen.UpdateConfig(ctx, settings))
	s.rm = makePatchDistroByID()
}

func (s *DistroPatchByIDSuite) TestParse() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"ssh_options":["StrictHostKeyChecking=no","BatchMode=yes","ConnectTimeout=10"]}`)
	req, _ := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/distros/fedora8", bytes.NewBuffer(json))

	err := s.rm.Parse(ctx, req)
	s.NoError(err)
	s.Equal(json, s.rm.(*distroIDPatchHandler).body)
}

func (s *DistroPatchByIDSuite) TestRunValidSpawnAllowed() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"user_spawn_allowed": true}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.UserSpawnAllowed, true)
}

func (s *DistroPatchByIDSuite) TestRunValidProvider() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"provider": "mock"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Provider, utility.ToStringPtr("mock"))
}

func (s *DistroPatchByIDSuite) TestRunProviderSettingsList() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	allDistros, err := distro.AllDistros(ctx)
	s.NoError(err)
	distro1 := allDistros[0]
	s.Len(distro1.ProviderSettingsList, 1)
	doc := distro1.ProviderSettingsList[0].Copy()
	doc = doc.Set(birch.EC.Double("bid_price", 0.15))
	doc = doc.Set(birch.EC.String("security_group", "password123"))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	temp := distro.Distro{Id: h.distroID, ProviderSettingsList: []*birch.Document{doc}}
	bytes, err := json.Marshal(temp)
	s.NoError(err)
	h.body = bytes
	temp2 := distro.Distro{}
	s.NoError(json.Unmarshal(bytes, &temp2))

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)

	s.Require().Len(apiDistro.ProviderSettingsList, 1)
	doc = apiDistro.ProviderSettingsList[0]
	mappedDoc, ok := doc.Lookup("mount_points").MutableArray().Lookup(0).MutableDocumentOK()
	s.True(ok)
	s.Equal(mappedDoc.Lookup("device_name").StringValue(), "/dev/xvdb")
	s.Equal(mappedDoc.Lookup("virtual_name").StringValue(), "ephemeral0")
	s.Equal(doc.Lookup("bid_price").Double(), 0.15)
	s.Equal(doc.Lookup("instance_type").StringValue(), "m3.large")
	s.Equal(doc.Lookup("key_name").StringValue(), "mci")
	s.Equal(doc.Lookup("security_group").StringValue(), "password123")
	s.Equal(doc.Lookup("ami").StringValue(), "ami-2814683f")
}

func (s *DistroPatchByIDSuite) TestRunValidArch() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"arch": "linux_amd64"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Arch, utility.ToStringPtr("linux_amd64"))
}

func (s *DistroPatchByIDSuite) TestRunValidWorkDir() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"work_dir": "/tmp"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.WorkDir, utility.ToStringPtr("/tmp"))
}

func (s *DistroPatchByIDSuite) TestRunValidHostAllocatorSettingsMaximumHosts() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"host_allocator_settings": {"maximum_hosts": 50}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.HostAllocatorSettings.MaximumHosts, 50)
}

func (s *DistroPatchByIDSuite) TestRunValidSetupAsSudo() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"setup_as_sudo": false}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.SetupAsSudo, false)
}

func (s *DistroPatchByIDSuite) TestRunValidSetup() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"setup": "New Set-up string"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Setup, utility.ToStringPtr("New Set-up string"))
}

func (s *DistroPatchByIDSuite) TestRunValidUser() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"user": "user101"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.User, utility.ToStringPtr("user101"))
}

func (s *DistroPatchByIDSuite) TestRunValidSSHKey() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"ssh_key": "New SSH Key"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.SSHKey, utility.ToStringPtr("New SSH Key"))
}

func (s *DistroPatchByIDSuite) TestRunValidSSHOptions() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"ssh_options":["BatchMode=no"]}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.SSHOptions, []string{"BatchMode=no"})
}

func (s *DistroPatchByIDSuite) TestRunValidExpansions() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"expansions": [{"key": "key1", "value": "value1"}]}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	expansion := restModel.APIExpansion{Key: utility.ToStringPtr("key1"), Value: utility.ToStringPtr("value1")}
	s.Equal(apiDistro.Expansions, []restModel.APIExpansion{expansion})
}

func (s *DistroPatchByIDSuite) TestRunValidDisabled() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"disabled": true}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Disabled, true)
}

func (s *DistroPatchByIDSuite) TestRunValidContainer() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"container_pool": ""}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.ContainerPool, utility.ToStringPtr(""))
	s.Equal(apiDistro.PlannerSettings.Version, utility.ToStringPtr("legacy"))
}

func (s *DistroPatchByIDSuite) TestRunInvalidEmptyStringValues() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"arch": "","user": "","work_dir": "","ssh_key": "","provider": "mock"}`)
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
	}

	error := (resp.Data()).(gimlet.ErrorResponse)
	for _, err := range errors {
		s.Contains(error.Message, err)
	}
}

func (s *DistroPatchByIDSuite) TestRunValidPlannerSettingsVersion() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"planner_settings": {"version": "tunable"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(utility.ToStringPtr("tunable"), apiDistro.PlannerSettings.Version)
}

func (s *DistroPatchByIDSuite) TestRunInvalidPlannerSettingsVersion() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"finder_settings": {"version": "legacy"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)
	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(utility.ToStringPtr("legacy"), apiDistro.PlannerSettings.Version)
}

func (s *DistroPatchByIDSuite) TestRunValidBootstrapMethod() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"bootstrap_settings": {"method": "legacy-ssh"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusOK)

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(utility.ToStringPtr(distro.BootstrapMethodLegacySSH), apiDistro.BootstrapSettings.Method)
}

func (s *DistroPatchByIDSuite) TestRunInvalidBootstrapMethod() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"bootstrap_settings": {"method": "foobar"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunValidCommunicationMethod() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"bootstrap_settings": {"communication": "legacy-ssh"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(utility.ToStringPtr(distro.CommunicationMethodLegacySSH), apiDistro.BootstrapSettings.Communication)
}

func (s *DistroPatchByIDSuite) TestRunInvalidCommunicationMethod() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(`{"bootstrap_settings": {"communication": "foobar"}}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunValidBootstrapAndCommunicationMethods() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(fmt.Sprintf(
		`{"bootstrap_settings": {"method": "%s", "communication": "%s"}}`,
		distro.BootstrapMethodLegacySSH, distro.CommunicationMethodLegacySSH))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())

	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(utility.ToStringPtr(distro.BootstrapMethodLegacySSH), apiDistro.BootstrapSettings.Method)
	s.Equal(utility.ToStringPtr(distro.CommunicationMethodLegacySSH), apiDistro.BootstrapSettings.Communication)
}

func (s *DistroPatchByIDSuite) TestRunInvalidBootstrapAndCommunicationMethods() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(fmt.Sprintf(
		`{"bootstrap_settings": {"method": "%s", "communication": "%s"}}`,
		distro.BootstrapMethodUserData, distro.CommunicationMethodLegacySSH))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *DistroPatchByIDSuite) TestRunMissingNonLegacyBootstrapSettings() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(fmt.Sprintf(
		`{"bootstrap_settings": {"method": "%s", "communication": "%s"}}`,
		distro.BootstrapMethodUserData, distro.CommunicationMethodSSH))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusBadRequest, resp.Status())
	err := (resp.Data()).(gimlet.ErrorResponse)
	s.Contains(err.Message, "client directory cannot be empty for non-legacy bootstrapping")
	s.Contains(err.Message, "Jasper binary directory cannot be empty for non-legacy bootstrapping")
	s.Contains(err.Message, "Jasper credentials path cannot be empty for non-legacy bootstrapping")
	s.Contains(err.Message, "client directory cannot be empty")
}

func (s *DistroPatchByIDSuite) TestRunValidNonLegacyBootstrapSettings() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	json := []byte(fmt.Sprintf(
		`{"bootstrap_settings": {
			"method": "%s",
			"communication": "%s",
			"client_dir": "/client_dir",
			"jasper_binary_dir": "/jasper_binary_dir",
			"jasper_credentials_path": "/jasper_credentials_path",
			"shell_path": "/shell_path",
			"root_dir": "/root_dir"
		}
	}`, distro.BootstrapMethodUserData, distro.CommunicationMethodSSH))
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())
	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(utility.ToStringPtr(distro.BootstrapMethodUserData), apiDistro.BootstrapSettings.Method)
	s.Equal(utility.ToStringPtr(distro.CommunicationMethodSSH), apiDistro.BootstrapSettings.Communication)
	s.Equal(utility.ToStringPtr("/client_dir"), apiDistro.BootstrapSettings.ClientDir)
	s.Equal(utility.ToStringPtr("/jasper_binary_dir"), apiDistro.BootstrapSettings.JasperBinaryDir)
	s.Equal(utility.ToStringPtr("/jasper_credentials_path"), apiDistro.BootstrapSettings.JasperCredentialsPath)
	s.Equal(utility.ToStringPtr("/shell_path"), apiDistro.BootstrapSettings.ShellPath)
	s.Equal(utility.ToStringPtr("/root_dir"), apiDistro.BootstrapSettings.RootDir)
}

func (s *DistroPatchByIDSuite) TestValidFindAndReplaceFullDocument() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	docToWrite := []byte(
		`{
				"arch" : "linux_amd64",
				"work_dir" : "~/data/mci",
				"host_allocator_settings": {
					"maximum_hosts": 20
				},
				"provider" : "mock",
				"provider_settings" : [
					{
						"mount_points" : [{
							"device_name" : "~/dev/xvdb",
							"virtual_name" : "~ephemeral0"
						}],
						"ami" : "~ami-2814683f",
						"bid_price" : 0.1,
						"instance_type" : "~m3.large",
						"key_name" : "~mci",
						"security_group" : "~mci",
						"region" : "us-east-1"
					},
					{
						"mount_points" : [{
							"device_name" : "~/dev/xvdb",
							"virtual_name" : "~ephemeral0"
						}],
						"ami" : "~ami-different",
						"bid_price" : 1.0,
						"instance_type" : "~m3.small",
						"key_name" : "icm",
						"security_group" : "icm",
						"region" : "us-west-2"
					}],
				"setup_as_sudo" : false,
				"setup" : "~Set-up script",
				"user" : "~root",
				"bootstrap_settings": {
					"method": "legacy-ssh",
					"communication": "legacy-ssh",
					"jasper_binary_dir": "/oldUsr/local/bin",
					"jasper_credentials_path": "/etc/credentials",
					"client_dir": "/oldUsr/bin",
					"service_user": "service_user",
					"shell_path": "/oldUsr/bin/bash",
					"root_dir" : "/new/root/dir",
					"env": [{"key": "envKey", "value": "envValue"}],
					"resource_limits": {
						"num_files": 1,
						"num_processes": 2,
						"num_tasks": 3,
						"locked_memory": 4,
						"virtual_memory": 5
					},
					"precondition_scripts": [
						{
						"path": "/tmp/foo",
						"script": "echo foo"
						}
					]
				},
				"clone_method": "legacy-ssh",
				"ssh_key" : "New SSH Key",
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
	h.body = docToWrite

	resp := s.rm.Run(ctx)
	s.Equal(resp.Status(), http.StatusOK)
	s.NotNil(resp.Data())
	apiDistro, ok := (resp.Data()).(*restModel.APIDistro)
	s.Require().True(ok)
	s.Equal(apiDistro.Disabled, false)
	s.Equal(apiDistro.Name, utility.ToStringPtr("fedora8"))
	s.Equal(apiDistro.WorkDir, utility.ToStringPtr("~/data/mci"))
	s.Equal(apiDistro.HostAllocatorSettings.MaximumHosts, 20)
	s.Equal(apiDistro.Provider, utility.ToStringPtr("mock"))

	s.Require().Len(apiDistro.ProviderSettingsList, 2)
	doc := apiDistro.ProviderSettingsList[0]

	mountPoint := doc.Lookup("mount_points").MutableArray().Lookup(0).MutableDocument()
	s.Equal(mountPoint.Lookup("device_name").StringValue(), "~/dev/xvdb")
	s.Equal(mountPoint.Lookup("virtual_name").StringValue(), "~ephemeral0")
	s.Equal(doc.Lookup("ami").StringValue(), "~ami-2814683f")
	s.Equal(doc.Lookup("bid_price").Double(), 0.10)
	s.Equal(doc.Lookup("instance_type").StringValue(), "~m3.large")

	s.Equal(apiDistro.SetupAsSudo, false)
	s.Equal(apiDistro.Setup, utility.ToStringPtr("~Set-up script"))
	s.Equal(utility.ToStringPtr(distro.BootstrapMethodLegacySSH), apiDistro.BootstrapSettings.Method)
	s.Equal(utility.ToStringPtr(distro.CommunicationMethodLegacySSH), apiDistro.BootstrapSettings.Communication)
	s.Equal(utility.ToStringPtr(evergreen.CloneMethodLegacySSH), apiDistro.CloneMethod)
	s.Equal(utility.ToStringPtr("/oldUsr/bin"), apiDistro.BootstrapSettings.ClientDir)
	s.Equal(utility.ToStringPtr("/oldUsr/local/bin"), apiDistro.BootstrapSettings.JasperBinaryDir)
	s.Equal(utility.ToStringPtr("/etc/credentials"), apiDistro.BootstrapSettings.JasperCredentialsPath)
	s.Equal(utility.ToStringPtr("service_user"), apiDistro.BootstrapSettings.ServiceUser)
	s.Equal(utility.ToStringPtr("/oldUsr/bin/bash"), apiDistro.BootstrapSettings.ShellPath)
	s.Equal(utility.ToStringPtr("/new/root/dir"), apiDistro.BootstrapSettings.RootDir)
	s.Equal([]restModel.APIEnvVar{{Key: utility.ToStringPtr("envKey"), Value: utility.ToStringPtr("envValue")}}, apiDistro.BootstrapSettings.Env)
	s.Equal(1, apiDistro.BootstrapSettings.ResourceLimits.NumFiles)
	s.Equal(2, apiDistro.BootstrapSettings.ResourceLimits.NumProcesses)
	s.Equal(3, apiDistro.BootstrapSettings.ResourceLimits.NumTasks)
	s.Equal(4, apiDistro.BootstrapSettings.ResourceLimits.LockedMemoryKB)
	s.Equal(5, apiDistro.BootstrapSettings.ResourceLimits.VirtualMemoryKB)
	s.Require().Len(apiDistro.BootstrapSettings.PreconditionScripts, 1)
	s.Equal(utility.ToStringPtr("/tmp/foo"), apiDistro.BootstrapSettings.PreconditionScripts[0].Path)
	s.Equal(utility.ToStringPtr("echo foo"), apiDistro.BootstrapSettings.PreconditionScripts[0].Script)
	s.Equal(utility.ToStringPtr("~root"), apiDistro.User)
	s.Equal(utility.ToStringPtr("New SSH Key"), apiDistro.SSHKey)
	s.Equal([]string{"~StrictHostKeyChecking=no", "~BatchMode=no", "~ConnectTimeout=10"}, apiDistro.SSHOptions)
	s.False(apiDistro.UserSpawnAllowed)

	s.Equal(apiDistro.Expansions, []restModel.APIExpansion{
		{Key: utility.ToStringPtr("~decompress"), Value: utility.ToStringPtr("~tar zxvf")},
		{Key: utility.ToStringPtr("~ps"), Value: utility.ToStringPtr("~ps aux")},
		{Key: utility.ToStringPtr("~kill_pid"), Value: utility.ToStringPtr("~kill -- -$(ps opgid= %v)")},
		{Key: utility.ToStringPtr("~scons_prune_ratio"), Value: utility.ToStringPtr("~0.8")},
	})

	// no problem turning into settings object
	settings := &cloud.EC2ProviderSettings{}
	bytes, err := doc.MarshalBSON()
	s.NoError(err)
	s.NoError(bson.Unmarshal(bytes, settings))
	s.NotEmpty(settings)
	s.NotEqual(settings.Region, "")
	s.Require().Len(settings.MountPoints, 1)
	s.Equal(settings.MountPoints[0].DeviceName, "~/dev/xvdb")
	s.Equal(settings.MountPoints[0].VirtualName, "~ephemeral0")
}

func (s *DistroPatchByIDSuite) TestRunInvalidNameChange() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})
	json := []byte(`{"name": "Updated distro name"}`)
	h := s.rm.(*distroIDPatchHandler)
	h.distroID = "fedora8"
	h.body = json

	resp := s.rm.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(resp.Status(), http.StatusForbidden)

	err := resp.Data().(gimlet.ErrorResponse)
	s.Equal(fmt.Sprintf("distro name 'Updated distro name' is immutable so it cannot be renamed to '%s'", h.distroID), err.Message)
}

func getMockDistrosdata() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	distros := []*distro.Distro{
		{
			Id:      "fedora8",
			Arch:    "linux_amd64",
			WorkDir: "/data/mci",
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MaximumHosts: 30,
			},
			Provider: "mock",
			ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.Double("bid_price", 0.2),
				birch.EC.String("instance_type", "m3.large"),
				birch.EC.String("key_name", "mci"),
				birch.EC.String("security_group", "mci"),
				birch.EC.String("ami", "ami-2814683f"),
				birch.EC.Array("mount_points", birch.NewArray(
					birch.VC.Document(birch.NewDocument(
						birch.EC.String("device_name", "/dev/xvdb"),
						birch.EC.String("virtual_name", "ephemeral0"),
					)),
				)),
				birch.EC.Interface("mount_points", map[string]interface{}{
					"device_name":  "/dev/xvdb",
					"virtual_name": "ephemeral0"}),
			)},
			SetupAsSudo: true,
			Setup:       "Set-up script",
			User:        "root",
			SSHKey:      "SSH key string",
			SSHOptions: []string{
				"StrictHostKeyChecking=no",
				"BatchMode=yes",
				"ConnectTimeout=10"},
			SpawnAllowed: false,
			Expansions: []distro.Expansion{
				{
					Key:   "decompress",
					Value: "tar zxvf"},
				{
					Key:   "ps",
					Value: "ps aux"},
				{
					Key:   "kill_pid",
					Value: "kill -- -$(ps opgid= %v)"},
				{
					Key:   "scons_prune_ratio",
					Value: "0.8"},
			},
			Disabled:      false,
			ContainerPool: "",
		},
	}
	for _, d := range distros {
		err := d.Insert(ctx)
		if err != nil {
			return nil
		}
	}
	return nil
}

///////////////////////////////////////////////////////////////////////
//
// Tests for PATCH /rest/v2/distro/{distro}/execute

type distroExecuteSuite struct {
	rh     *distroExecuteHandler
	env    evergreen.Environment
	cancel context.CancelFunc

	suite.Suite
}

func TestDistroExecuteSuite(t *testing.T) {
	suite.Run(t, new(distroExecuteSuite))
}

func (s *distroExecuteSuite) SetupTest() {
	s.NoError(db.ClearCollections(host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.env = testutil.NewEnvironment(ctx, s.T())
	hostToAdd := host.Host{
		Id: "host1",
		Distro: distro.Distro{
			Id:      "distro1",
			Aliases: []string{"alias1"},
		},
	}
	s.Require().NoError(hostToAdd.Insert(ctx))
	h := makeDistroExecute(s.env)
	rh, ok := h.(*distroExecuteHandler)
	s.Require().True(ok)
	s.rh = rh
}

func (s *distroExecuteSuite) TearDownTest() {
	s.cancel()
}

func (s *distroExecuteSuite) TestParse() {
	ctx, _ := s.env.Context()

	body := []byte(`
	{
		"script": "echo foobar",
		"include_spawn_hosts": true,
		"include_task_hosts": true
	}`,
	)
	req, err := http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/distros/distro1/execute", bytes.NewBuffer(body))
	s.Require().NoError(err)
	s.NoError(s.rh.Parse(ctx, req))

	emptyBody := []byte(`{"script": ""}`)
	req, err = http.NewRequest(http.MethodPatch, "http://example.com/api/rest/v2/distros/distro1/execute", bytes.NewBuffer(emptyBody))
	s.Require().NoError(err)
	s.Error(s.rh.Parse(ctx, req))
}

func (s *distroExecuteSuite) TestRunWithDistroID() {
	s.rh.distro = "distro1"
	s.rh.opts = restModel.APIDistroScriptOptions{
		Script:           "echo foobar",
		IncludeTaskHosts: true,
	}

	ctx, _ := s.env.Context()
	resp := s.rh.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())
}

func (s *distroExecuteSuite) TestRunWithDistroAlias() {
	s.rh.distro = "alias1"
	s.rh.opts = restModel.APIDistroScriptOptions{
		Script:           "echo foobar",
		IncludeTaskHosts: true,
	}

	ctx, _ := s.env.Context()
	resp := s.rh.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())
}

func (s *distroExecuteSuite) TestRunWithNoHostSelector() {
	s.rh.distro = "alias1"

	ctx, _ := s.env.Context()
	resp := s.rh.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())
}

func (s *distroExecuteSuite) TestRunNonexistentDistro() {
	ctx := context.Background()
	s.rh.distro = "nonexistent"
	s.rh.opts.Script = "echo foobar"

	resp := s.rh.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusOK, resp.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests for GET /rest/v2/distros/{distro_id}/client_urls

type distroClientURLsGetSuite struct {
	rh     *distroClientURLsGetHandler
	env    evergreen.Environment
	cancel context.CancelFunc

	suite.Suite
}

func TestDistroClientURLsGetSuite(t *testing.T) {
	suite.Run(t, new(distroClientURLsGetSuite))
}

func (s *distroClientURLsGetSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	d := distro.Distro{Id: "distroID"}
	s.NoError(db.ClearCollections(distro.Collection))
	err := d.Insert(ctx)
	s.NoError(err)
	s.env = testutil.NewEnvironment(ctx, s.T())
	h := makeGetDistroClientURLs(s.env)
	rh, ok := h.(*distroClientURLsGetHandler)
	s.Require().True(ok)
	s.rh = rh
}

func (s *distroClientURLsGetSuite) TearDownTest() {
	s.cancel()
}

func (s *distroClientURLsGetSuite) TestRunWithDistroID() {
	s.rh.distroID = "distroID"
	ctx, _ := s.env.Context()
	resp := s.rh.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Require().NotNil(resp.Data())
	urls := resp.Data().([]string)
	s.NotEmpty(urls)
}

func (s *distroClientURLsGetSuite) TestRunNonexistentDistro() {
	ctx := context.Background()
	s.rh.distroID = "nonexistent"

	resp := s.rh.Run(ctx)
	s.NotNil(resp.Data())
	s.Equal(http.StatusNotFound, resp.Status())
}
