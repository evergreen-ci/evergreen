package distro

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func TestGenerateName(t *testing.T) {
	assert := assert.New(t)

	d := Distro{
		Provider: evergreen.ProviderNameStatic,
	}
	assert.Equal("static", d.Provider)

	d.Provider = evergreen.ProviderNameDocker
	match, err := regexp.MatchString("container-[0-9]+", d.GenerateName())
	assert.NoError(err)
	assert.True(match)

	d.Id = "test"
	d.Provider = "somethingcompletelydifferent"
	match, err = regexp.MatchString("evg-test-[0-9]+-[0-9]+", d.GenerateName())
	assert.NoError(err)
	assert.True(match)
}

func TestGenerateGceName(t *testing.T) {
	assert := assert.New(t)

	r, err := regexp.Compile("(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)")
	assert.NoError(err)
	d := Distro{Id: "name"}

	nameA := d.GenerateName()
	nameB := d.GenerateName()
	assert.True(r.Match([]byte(nameA)))
	assert.True(r.Match([]byte(nameB)))
	assert.NotEqual(nameA, nameB)

	d.Id = "!nv@lid N@m3*"
	invalidChars := d.GenerateName()
	assert.True(r.Match([]byte(invalidChars)))

	d.Id = strings.Repeat("abc", 10)
	tooManyChars := d.GenerateName()
	assert.True(r.Match([]byte(tooManyChars)))
}

func TestIsParent(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(Collection))
	assert.NoError(db.Clear(evergreen.ConfigCollection))

	conf := evergreen.ContainerPoolsConfig{
		Pools: []evergreen.ContainerPool{
			evergreen.ContainerPool{
				Distro:        "distro-1",
				Id:            "test-pool",
				MaxContainers: 100,
			},
		},
	}
	assert.NoError(conf.Set())

	settings, err := evergreen.GetConfig()
	assert.NoError(err)

	d1 := &Distro{
		Id: "distro-1",
	}
	d2 := &Distro{
		Id: "distro-2",
	}
	d3 := &Distro{
		Id:            "distro-3",
		ContainerPool: "test-pool",
	}
	assert.NoError(d1.Insert())
	assert.NoError(d2.Insert())
	assert.NoError(d3.Insert())

	assert.True(d1.IsParent(settings))
	assert.False(d2.IsParent(settings))
	assert.False(d3.IsParent(settings))
}

func TestValidateContainerPoolDistros(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(Collection))

	d1 := &Distro{
		Id: "valid-distro",
	}
	d2 := &Distro{
		Id:            "invalid-distro",
		ContainerPool: "test-pool-1",
	}
	assert.NoError(d1.Insert())
	assert.NoError(d2.Insert())

	testSettings := &evergreen.Settings{
		ContainerPools: evergreen.ContainerPoolsConfig{
			Pools: []evergreen.ContainerPool{
				evergreen.ContainerPool{
					Distro:        "valid-distro",
					Id:            "test-pool-1",
					MaxContainers: 100,
				},
				evergreen.ContainerPool{
					Distro:        "invalid-distro",
					Id:            "test-pool-2",
					MaxContainers: 100,
				},
				evergreen.ContainerPool{
					Distro:        "missing-distro",
					Id:            "test-pool-3",
					MaxContainers: 100,
				},
			},
		},
	}

	err := ValidateContainerPoolDistros(testSettings)
	assert.Contains(err.Error(), "container pool 'test-pool-2' has invalid distro 'invalid-distro'")
	assert.Contains(err.Error(), "error finding distro for container pool 'test-pool-3'")
}

func TestGetDistroIds(t *testing.T) {
	assert := assert.New(t)
	distros := DistroGroup{
		Distro{
			Id: "d1",
		},
		Distro{
			Id: "d2",
		},
		Distro{
			Id: "d3",
		},
	}
	ids := distros.GetDistroIds()
	assert.Equal([]string{"d1", "d2", "d3"}, ids)
}

func TestGetImageID(t *testing.T) {
	for _, test := range []struct {
		name           string
		provider       string
		key            string
		value          string
		expectedOutput string
		err            bool
		noKey          bool
		legacyOnly     bool
	}{
		{
			name:           "Ec2Auto",
			provider:       evergreen.ProviderNameEc2Auto,
			key:            "ami",
			value:          "imageID",
			expectedOutput: "imageID",
		},
		{
			name:           "Ec2OnDemand",
			provider:       evergreen.ProviderNameEc2OnDemand,
			key:            "ami",
			value:          "imageID",
			expectedOutput: "imageID",
		},
		{
			name:           "Ec2Spot",
			provider:       evergreen.ProviderNameEc2Spot,
			key:            "ami",
			value:          "imageID",
			expectedOutput: "imageID",
		},
		{
			name:           "Ec2Fleet",
			provider:       evergreen.ProviderNameEc2Fleet,
			key:            "ami",
			value:          "",
			expectedOutput: "",
		},
		{
			name:     "Ec2NoKey",
			provider: evergreen.ProviderNameEc2Fleet,
			noKey:    true,
			err:      true,
		},
		{
			name:           "Docker",
			provider:       evergreen.ProviderNameDocker,
			key:            "image_url",
			value:          "imageID",
			expectedOutput: "imageID",
		},
		{
			name:           "DockerMock",
			provider:       evergreen.ProviderNameDockerMock,
			key:            "image_url",
			value:          "imageID",
			expectedOutput: "imageID",
		},
		{
			name:           "Gce",
			provider:       evergreen.ProviderNameGce,
			key:            "image_name",
			value:          "imageID",
			expectedOutput: "imageID",
		},
		{
			name:     "Static",
			provider: evergreen.ProviderNameStatic,
			noKey:    true,
		},
		{
			name:     "Openstack",
			provider: evergreen.ProviderNameOpenstack,
			noKey:    true,
		},
		{
			name:           "Vsphere",
			provider:       evergreen.ProviderNameVsphere,
			key:            "template",
			value:          "imageID",
			expectedOutput: "imageID",
		},
		{
			name:     "Mock",
			provider: evergreen.ProviderNameMock,
			noKey:    true,
		},
		{
			name:     "UnknownProvider",
			provider: "unknown",
			noKey:    true,
			err:      true,
		},
		{
			name:       "InvalidKey",
			provider:   evergreen.ProviderNameEc2Auto,
			key:        "abi",
			value:      "imageID",
			err:        true,
			legacyOnly: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			providerSettings := birch.NewDocument()
			if !test.noKey {
				providerSettings.Set(birch.EC.String(test.key, test.value))
			}
			d := Distro{Provider: test.provider, ProviderSettingsList: []*birch.Document{providerSettings}}
			output1, err1 := d.GetImageID()

			doc := birch.NewDocument(birch.EC.String(test.key, test.value))
			d = Distro{Provider: test.provider, ProviderSettingsList: []*birch.Document{doc}}
			output2, err2 := d.GetImageID()
			assert.Equal(t, output1, test.expectedOutput)
			assert.Equal(t, output2, test.expectedOutput)
			if test.err {
				assert.Error(t, err1)
				if !test.legacyOnly {
					assert.Error(t, err2)
				}
			} else {
				assert.NoError(t, err1)
				assert.NoError(t, err2)
			}
		})
	}
}

func TestGetResolvedHostAllocatorSettings(t *testing.T) {
	d0 := Distro{
		Id: "distro0",
		HostAllocatorSettings: HostAllocatorSettings{
			Version:                "",
			MinimumHosts:           4,
			MaximumHosts:           10,
			AcceptableHostIdleTime: 0,
			RoundingRule:           evergreen.HostAllocatorRoundDefault,
			FeedbackRule:           evergreen.HostAllocatorUseDefaultFeedback,
			HostsOverallocatedRule: evergreen.HostsOverallocatedUseDefault,
		},
	}
	config0 := evergreen.SchedulerConfig{
		TaskFinder:                    "legacy",
		HostAllocator:                 evergreen.HostAllocatorUtilization,
		HostAllocatorRoundingRule:     evergreen.HostAllocatorRoundDown,
		HostAllocatorFeedbackRule:     evergreen.HostAllocatorNoFeedback,
		HostsOverallocatedRule:        evergreen.HostsOverallocatedIgnore,
		FutureHostFraction:            .1,
		CacheDurationSeconds:          60,
		Planner:                       evergreen.PlannerVersionLegacy,
		TargetTimeSeconds:             112358,
		AcceptableHostIdleTimeSeconds: 123,
		GroupVersions:                 false,
		PatchFactor:                   50,
		PatchTimeInQueueFactor:        12,
		CommitQueueFactor:             50,
		MainlineTimeInQueueFactor:     10,
		ExpectedRuntimeFactor:         7,
	}

	settings0 := &evergreen.Settings{Scheduler: config0}

	resolved0, err := d0.GetResolvedHostAllocatorSettings(settings0)
	assert.NoError(t, err)
	// Fallback to the SchedulerConfig.HostAllocator as HostAllocatorSettings.Version is an empty string.
	assert.Equal(t, evergreen.HostAllocatorUtilization, resolved0.Version)
	assert.Equal(t, 4, resolved0.MinimumHosts)
	assert.Equal(t, 10, resolved0.MaximumHosts)
	assert.Equal(t, evergreen.HostAllocatorRoundDown, resolved0.RoundingRule)
	assert.Equal(t, evergreen.HostAllocatorNoFeedback, resolved0.FeedbackRule)
	assert.Equal(t, evergreen.HostsOverallocatedIgnore, resolved0.HostsOverallocatedRule)
	// Fallback to the SchedulerConfig.AcceptableHostIdleTimeSeconds as HostAllocatorSettings.AcceptableHostIdleTime is equal to 0.
	assert.Equal(t, time.Duration(123)*time.Second, resolved0.AcceptableHostIdleTime)

	// test distro-first override when RoundingRule is not HostAllocatorRoundDefault
	d0.HostAllocatorSettings.RoundingRule = evergreen.HostAllocatorRoundUp
	resolved0, err = d0.GetResolvedHostAllocatorSettings(settings0)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.HostAllocatorRoundUp, resolved0.RoundingRule)

	d0.HostAllocatorSettings.FeedbackRule = evergreen.HostAllocatorWaitsOverThreshFeedback
	resolved0, err = d0.GetResolvedHostAllocatorSettings(settings0)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.HostAllocatorWaitsOverThreshFeedback, resolved0.FeedbackRule)

	d0.HostAllocatorSettings.HostsOverallocatedRule = evergreen.HostsOverallocatedTerminate
	resolved0, err = d0.GetResolvedHostAllocatorSettings(settings0)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.HostsOverallocatedTerminate, resolved0.HostsOverallocatedRule)
}

func TestGetResolvedPlannerSettings(t *testing.T) {
	d0 := Distro{
		Id: "distro0",
		PlannerSettings: PlannerSettings{
			Version:                   "",
			TargetTime:                0,
			GroupVersions:             nil,
			PatchFactor:               0,
			PatchTimeInQueueFactor:    0,
			CommitQueueFactor:         0,
			MainlineTimeInQueueFactor: 0,
			ExpectedRuntimeFactor:     0,
			GenerateTaskFactor:        0,
		},
	}
	config0 := evergreen.SchedulerConfig{
		TaskFinder:                    "legacy",
		HostAllocator:                 evergreen.HostAllocatorUtilization,
		FutureHostFraction:            .1,
		CacheDurationSeconds:          60,
		Planner:                       evergreen.PlannerVersionLegacy,
		TargetTimeSeconds:             112358,
		AcceptableHostIdleTimeSeconds: 132134,
		GroupVersions:                 false,
		PatchFactor:                   50,
		PatchTimeInQueueFactor:        12,
		CommitQueueFactor:             50,
		MainlineTimeInQueueFactor:     10,
		ExpectedRuntimeFactor:         7,
		GenerateTaskFactor:            20,
		StepbackTaskFactor:            40,
	}

	settings0 := &evergreen.Settings{Scheduler: config0}

	resolved0, err := d0.GetResolvedPlannerSettings(settings0)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.PlannerVersionLegacy, resolved0.Version)
	assert.Equal(t, time.Duration(112358)*time.Second, resolved0.TargetTime)
	// Fallback to the SchedulerConfig.GroupVersions as PlannerSettings.GroupVersions is nil.
	assert.Equal(t, false, *resolved0.GroupVersions)
	// Fallback to the SchedulerConfig.PatchFactor as PlannerSettings.PatchFactor is is equal to 0.
	assert.EqualValues(t, 50, resolved0.PatchFactor)
	// Fallback to the SchedulerConfig.PatchTimeInQueueFactor as PlannerSettings.PatchTimeInQueueFactor is equal to 0.
	assert.EqualValues(t, 12, resolved0.PatchTimeInQueueFactor)
	// Fallback to the SchedulerConfig.CommitQueueFactor as PlannerSettings.CommitQueueFactor is equal to 0.
	assert.EqualValues(t, 50, resolved0.CommitQueueFactor)
	// Fallback to the SchedulerConfig.MainlineTimeInQueueFactor as PlannerSettings.MainlineTimeInQueueFactor is equal to 0.
	assert.EqualValues(t, 10, resolved0.MainlineTimeInQueueFactor)
	// Fallback to the SchedulerConfig.ExpectedRuntimeFactor as PlannerSettings.ExpectedRunTimeFactor is equal to 0.
	assert.EqualValues(t, 7, resolved0.ExpectedRuntimeFactor)
	assert.EqualValues(t, 20, resolved0.GenerateTaskFactor)
	assert.EqualValues(t, 40, resolved0.StepbackTaskFactor)

	d1 := Distro{
		Id: "distro1",
		PlannerSettings: PlannerSettings{
			Version:                   evergreen.PlannerVersionTunable,
			TargetTime:                98765000000000,
			GroupVersions:             utility.TruePtr(),
			PatchFactor:               25,
			PatchTimeInQueueFactor:    0,
			CommitQueueFactor:         0,
			MainlineTimeInQueueFactor: 0,
			ExpectedRuntimeFactor:     0,
			GenerateTaskFactor:        0,
		},
	}
	config1 := evergreen.SchedulerConfig{
		TaskFinder:                    "legacy",
		HostAllocator:                 evergreen.HostAllocatorUtilization,
		FutureHostFraction:            .1,
		CacheDurationSeconds:          60,
		Planner:                       evergreen.PlannerVersionLegacy,
		TargetTimeSeconds:             10,
		AcceptableHostIdleTimeSeconds: 60,
		GroupVersions:                 false,
		PatchFactor:                   50,
		PatchTimeInQueueFactor:        0,
		CommitQueueFactor:             0,
		MainlineTimeInQueueFactor:     0,
		ExpectedRuntimeFactor:         0,
		GenerateTaskFactor:            0,
	}

	settings1 := &evergreen.Settings{Scheduler: config1}

	// d1.PlannerSettings' field values are all set and valid, so there is no need to fallback on any SchedulerConfig field values
	resolved1, err := d1.GetResolvedPlannerSettings(settings1)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.PlannerVersionTunable, resolved1.Version)
	assert.Equal(t, time.Duration(98765)*time.Second, resolved1.TargetTime)
	assert.Equal(t, true, *resolved1.GroupVersions)
	assert.EqualValues(t, 25, resolved1.PatchFactor)
	assert.EqualValues(t, 0, resolved1.PatchTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved1.CommitQueueFactor)
	assert.EqualValues(t, 0, resolved1.MainlineTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved1.ExpectedRuntimeFactor)
	assert.EqualValues(t, 0, resolved1.GenerateTaskFactor)

	ps := &PlannerSettings{
		Version:                   "",
		TargetTime:                0,
		GroupVersions:             nil,
		PatchFactor:               19,
		PatchTimeInQueueFactor:    0,
		CommitQueueFactor:         0,
		MainlineTimeInQueueFactor: 0,
		ExpectedRuntimeFactor:     0,
		GenerateTaskFactor:        0,
	}
	d2 := Distro{
		Id:              "distro2",
		PlannerSettings: *ps,
	}
	config2 := evergreen.SchedulerConfig{
		TaskFinder:                    "",
		HostAllocator:                 "",
		FutureHostFraction:            .1,
		CacheDurationSeconds:          60,
		Planner:                       evergreen.PlannerVersionLegacy,
		TargetTimeSeconds:             12345,
		AcceptableHostIdleTimeSeconds: 67890,
		GroupVersions:                 false,
		PatchFactor:                   0,
		PatchTimeInQueueFactor:        0,
		CommitQueueFactor:             0,
		MainlineTimeInQueueFactor:     0,
		ExpectedRuntimeFactor:         0,
		GenerateTaskFactor:            0,
	}
	settings2 := &evergreen.Settings{Scheduler: config2}

	resolved2, err := d2.GetResolvedPlannerSettings(settings2)
	require.NoError(t, err)
	// d2.PlannerSetting.Version is an empty string -- fallback on the SchedulerConfig.PlannerVersion value
	assert.Equal(t, evergreen.PlannerVersionLegacy, resolved2.Version)

	// d2.PlannerSetting.TargetTime is 0 -- fallback on the equivalent SchedulerConfig field value
	assert.Equal(t, time.Duration(12345)*time.Second, resolved2.TargetTime)
	// d2.PlannerSetting.GroupVersions is nil -- fallback on the SchedulerConfig.PlannerVersion.GroupVersions value
	assert.Equal(t, false, *resolved2.GroupVersions)
	assert.EqualValues(t, 19, resolved2.PatchFactor)
	assert.EqualValues(t, 0, resolved2.PatchTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved2.CommitQueueFactor)
	assert.EqualValues(t, 0, resolved2.MainlineTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved2.ExpectedRuntimeFactor)
	assert.EqualValues(t, 0, resolved2.GenerateTaskFactor)
}

func TestAddPermissions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(user.Collection, Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	u := user.DBUser{
		Id: "me",
	}
	require.NoError(t, u.Insert())
	d := Distro{
		Id: "myDistro",
	}
	require.NoError(t, d.Add(&u))

	rm := env.RoleManager()
	scope, err := rm.FindScopeForResources(evergreen.DistroResourceType, d.Id)
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	role, err := rm.FindRoleWithPermissions(evergreen.DistroResourceType, []string{d.Id}, map[string]int{
		evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value,
		evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
	})
	assert.NoError(t, err)
	assert.NotNil(t, role)
	dbUser, err := user.FindOneById(u.Id)
	assert.NoError(t, err)
	assert.Contains(t, dbUser.Roles(), "admin_distro_myDistro")
}

func TestLogDistroModifiedWithDistroData(t *testing.T) {
	assert.NoError(t, db.ClearCollections(event.AllLogCollection))

	d := Distro{
		Id:       "rainbow-lollipop",
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{
			birch.NewDocument().Set(birch.EC.String("ami", "ami-123456")),
			birch.NewDocument().Set(birch.EC.SliceString("groups", []string{"group1", "group2"})),
		},
	}
	event.LogDistroModified(d.Id, "user1", d.NewDistroData())
	eventsForDistro, err := event.Find(event.AllLogCollection, event.DistroEventsInOrder(d.Id))
	assert.NoError(t, err)
	require.Len(t, eventsForDistro, 1)
	eventData, ok := eventsForDistro[0].Data.(*event.DistroEventData)
	assert.True(t, ok)
	assert.Equal(t, "user1", eventData.UserId)
	assert.NotNil(t, eventData.Data)

	data := DistroData{}
	body, err := bson.Marshal(eventData.Data)
	assert.NoError(t, err)
	assert.NoError(t, bson.Unmarshal(body, &data))
	require.NotNil(t, data)
	assert.Equal(t, d.Id, data.Distro.Id)
	assert.Equal(t, d.Provider, data.Distro.Provider)
	assert.Nil(t, data.Distro.ProviderSettingsList)
	require.Len(t, data.ProviderSettingsMap, 2)
	assert.EqualValues(t, d.ProviderSettingsList[0].ExportMap(), data.ProviderSettingsMap[0])
	assert.EqualValues(t, d.ProviderSettingsList[1].ExportMap(), data.ProviderSettingsMap[1])
}

func TestS3ClientURL(t *testing.T) {
	d := Distro{Arch: evergreen.ArchWindowsAmd64}
	settings := &evergreen.Settings{
		HostInit: evergreen.HostInitConfig{
			S3BaseURL: "https://foo.com",
		},
		ClientBinariesDir: "clients",
	}

	expected := fmt.Sprintf("https://foo.com/%s/windows_amd64/evergreen.exe", evergreen.BuildRevision)
	assert.Equal(t, expected, d.S3ClientURL(settings))

	d.Arch = evergreen.ArchLinuxAmd64
	expected = fmt.Sprintf("https://foo.com/%s/linux_amd64/evergreen", evergreen.BuildRevision)
	assert.Equal(t, expected, d.S3ClientURL(settings))
}

func TestClientURL(t *testing.T) {
	d := Distro{Arch: evergreen.ArchWindowsAmd64}
	settings := &evergreen.Settings{
		ApiUrl:            "www.example.com",
		ClientBinariesDir: "clients",
	}

	expected := "www.example.com/clients/windows_amd64/evergreen.exe"
	assert.Equal(t, expected, d.ClientURL(settings))

	d.Arch = evergreen.ArchLinuxAmd64
	expected = "www.example.com/clients/linux_amd64/evergreen"
	assert.Equal(t, expected, d.ClientURL(settings))
}
