package distro

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindDistroById(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testConfig := testutil.TestConfig()
	assert := assert.New(t)
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	assert.NoError(err)
	require.NotNil(t, session)
	defer session.Close()

	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())

	id := fmt.Sprintf("distro_%d", rand.Int())
	d := &Distro{
		Id: id,
	}
	assert.NoError(d.Insert(ctx))
	found, err := FindOneId(ctx, id)
	assert.NoError(err)
	assert.Equal(found.Id, id, "The _ids should match")
	assert.NotEqual(found.Id, -1, "The _ids should not match")
}

func TestFindAllDistros(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testConfig := testutil.TestConfig()
	assert := assert.New(t)
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	assert.NoError(err)
	require.NotNil(t, session)
	defer session.Close()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())

	numDistros := 10
	for i := 0; i < numDistros; i++ {
		d := &Distro{
			Id: fmt.Sprintf("distro_%d", rand.Int()),
		}
		assert.NoError(d.Insert(ctx))
	}

	found, err := AllDistros(ctx)
	assert.NoError(err)
	assert.Len(found, numDistros)
}

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

func TestIsParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.Clear(Collection))
	assert.NoError(db.Clear(evergreen.ConfigCollection))

	conf := evergreen.ContainerPoolsConfig{
		Pools: []evergreen.ContainerPool{
			{
				Distro:        "distro-1",
				Id:            "test-pool",
				MaxContainers: 100,
			},
		},
	}
	assert.NoError(conf.Set(ctx))

	settings, err := evergreen.GetConfig(ctx)
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
	assert.NoError(d1.Insert(ctx))
	assert.NoError(d2.Insert(ctx))
	assert.NoError(d3.Insert(ctx))

	assert.True(d1.IsParent(settings))
	assert.False(d2.IsParent(settings))
	assert.False(d3.IsParent(settings))
}

func TestGetDefaultAMI(t *testing.T) {
	d := Distro{
		Id: "d1",
		ProviderSettingsList: []*birch.Document{
			birch.NewDocument(
				birch.EC.String("ami", "ami-1234"),
				birch.EC.String("region", "us-west-1"),
			),
		},
	}
	assert.Empty(t, d.GetDefaultAMI(), "")

	d.ProviderSettingsList = append(d.ProviderSettingsList, birch.NewDocument(
		birch.EC.String("ami", "ami-5678"),
		birch.EC.String("region", evergreen.DefaultEC2Region),
	))
	assert.Equal(t, "ami-5678", d.GetDefaultAMI())
}

func TestValidateContainerPoolDistros(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.Clear(Collection))

	d1 := &Distro{
		Id: "valid-distro",
	}
	d2 := &Distro{
		Id:            "invalid-distro",
		ContainerPool: "test-pool-1",
	}
	assert.NoError(d1.Insert(ctx))
	assert.NoError(d2.Insert(ctx))

	testSettings := &evergreen.Settings{
		ContainerPools: evergreen.ContainerPoolsConfig{
			Pools: []evergreen.ContainerPool{
				{
					Distro:        "valid-distro",
					Id:            "test-pool-1",
					MaxContainers: 100,
				},
				{
					Distro:        "invalid-distro",
					Id:            "test-pool-2",
					MaxContainers: 100,
				},
				{
					Distro:        "missing-distro",
					Id:            "test-pool-3",
					MaxContainers: 100,
				},
			},
		},
	}

	err := ValidateContainerPoolDistros(ctx, testSettings)
	require.NotNil(t, err)
	assert.Contains(err.Error(), "container pool 'test-pool-2' has invalid distro 'invalid-distro'")
	assert.Contains(err.Error(), "distro not found for container pool 'test-pool-3'")
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
			name:           "Ec2OnDemand",
			provider:       evergreen.ProviderNameEc2OnDemand,
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
			name:     "Static",
			provider: evergreen.ProviderNameStatic,
			noKey:    true,
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
			provider:   evergreen.ProviderNameEc2Fleet,
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
			assert.Equal(t, test.expectedOutput, output1)
			assert.Equal(t, test.expectedOutput, output2)
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
		TargetTimeSeconds:             112358,
		AcceptableHostIdleTimeSeconds: 123,
		GroupVersions:                 false,
		PatchFactor:                   50,
		PatchTimeInQueueFactor:        12,
		CommitQueueFactor:             50,
		MainlineTimeInQueueFactor:     10,
		ExpectedRuntimeFactor:         7,
	}
	releaseModeConfig := evergreen.ReleaseModeConfig{
		DistroMaxHostsFactor:    2.0,
		IdleTimeSecondsOverride: 300,
	}
	serviceFlags := evergreen.ServiceFlags{
		ReleaseModeDisabled: true,
	}
	settings0 := &evergreen.Settings{Scheduler: config0, ReleaseMode: releaseModeConfig, ServiceFlags: serviceFlags}

	resolved0, err := d0.GetResolvedHostAllocatorSettings(settings0)
	assert.NoError(t, err)
	// Fallback to the SchedulerConfig.HostAllocator as HostAllocatorSettings.Version is an empty string.
	assert.Equal(t, evergreen.HostAllocatorUtilization, resolved0.Version)
	assert.Equal(t, 4, resolved0.MinimumHosts)
	assert.Equal(t, 10, resolved0.MaximumHosts) // Ignore release mode when disabled.
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

	settings1 := &evergreen.Settings{Scheduler: config0, ReleaseMode: releaseModeConfig}
	resolved1, err := d0.GetResolvedHostAllocatorSettings(settings1)
	assert.NoError(t, err)

	// Factor in release mode when enabled.
	assert.Equal(t, 20, resolved1.MaximumHosts)
	assert.Equal(t, time.Duration(300)*time.Second, resolved1.AcceptableHostIdleTime)
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
			NumDependentsFactor:       0,
		},
	}
	config0 := evergreen.SchedulerConfig{
		TaskFinder:                    "legacy",
		HostAllocator:                 evergreen.HostAllocatorUtilization,
		FutureHostFraction:            .1,
		CacheDurationSeconds:          60,
		TargetTimeSeconds:             112358,
		AcceptableHostIdleTimeSeconds: 132134,
		GroupVersions:                 false,
		PatchFactor:                   50,
		PatchTimeInQueueFactor:        12,
		CommitQueueFactor:             50,
		MainlineTimeInQueueFactor:     10,
		ExpectedRuntimeFactor:         7,
		GenerateTaskFactor:            20,
		NumDependentsFactor:           10,
		StepbackTaskFactor:            40,
	}
	releaseConfig := evergreen.ReleaseModeConfig{
		TargetTimeSecondsOverride: 1200,
	}
	serviceFlags := evergreen.ServiceFlags{ReleaseModeDisabled: true}

	settings0 := &evergreen.Settings{Scheduler: config0, ReleaseMode: releaseConfig, ServiceFlags: serviceFlags}

	resolved0, err := d0.GetResolvedPlannerSettings(settings0)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.PlannerVersionTunable, resolved0.Version)
	// Ignore release mode when disabled.
	assert.Equal(t, time.Duration(112358)*time.Second, resolved0.TargetTime)
	// Fallback to the SchedulerConfig.GroupVersions as PlannerSettings.GroupVersions is nil.
	assert.False(t, *resolved0.GroupVersions)
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
	//nolint:testifylint // We expect it to be exactly 10.
	assert.EqualValues(t, 10, resolved0.NumDependentsFactor)
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
			NumDependentsFactor:       0,
		},
	}
	config1 := evergreen.SchedulerConfig{
		TaskFinder:                    "legacy",
		HostAllocator:                 evergreen.HostAllocatorUtilization,
		FutureHostFraction:            .1,
		CacheDurationSeconds:          60,
		TargetTimeSeconds:             10,
		AcceptableHostIdleTimeSeconds: 60,
		GroupVersions:                 false,
		PatchFactor:                   50,
		PatchTimeInQueueFactor:        0,
		CommitQueueFactor:             0,
		MainlineTimeInQueueFactor:     0,
		ExpectedRuntimeFactor:         0,
		GenerateTaskFactor:            0,
		NumDependentsFactor:           0,
	}

	settings1 := &evergreen.Settings{Scheduler: config1, ReleaseMode: releaseConfig}

	// d1.PlannerSettings' field values are all set and valid, so there is no need to fallback on any SchedulerConfig field values
	resolved1, err := d1.GetResolvedPlannerSettings(settings1)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.PlannerVersionTunable, resolved1.Version)
	// Release mode value is used because release mode is enabled.
	assert.Equal(t, time.Duration(1200)*time.Second, resolved1.TargetTime)
	assert.True(t, *resolved1.GroupVersions)
	assert.EqualValues(t, 25, resolved1.PatchFactor)
	assert.EqualValues(t, 0, resolved1.PatchTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved1.CommitQueueFactor)
	assert.EqualValues(t, 0, resolved1.MainlineTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved1.ExpectedRuntimeFactor)
	assert.EqualValues(t, 0, resolved1.GenerateTaskFactor)
	//nolint:testifylint // We expect it to be exactly 0.
	assert.EqualValues(t, 0, resolved1.NumDependentsFactor)

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
		NumDependentsFactor:       0,
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
		TargetTimeSeconds:             12345,
		AcceptableHostIdleTimeSeconds: 67890,
		GroupVersions:                 false,
		PatchFactor:                   0,
		PatchTimeInQueueFactor:        0,
		CommitQueueFactor:             0,
		MainlineTimeInQueueFactor:     0,
		ExpectedRuntimeFactor:         0,
		GenerateTaskFactor:            0,
		NumDependentsFactor:           0,
	}
	settings2 := &evergreen.Settings{Scheduler: config2}

	resolved2, err := d2.GetResolvedPlannerSettings(settings2)
	require.NoError(t, err)

	// d2.PlannerSetting.TargetTime is 0 -- fallback on the equivalent SchedulerConfig field value
	assert.Equal(t, time.Duration(12345)*time.Second, resolved2.TargetTime)
	// d2.PlannerSetting.GroupVersions is nil -- fallback on the SchedulerConfig.PlannerVersion.GroupVersions value
	assert.False(t, *resolved2.GroupVersions)
	assert.Equal(t, evergreen.PlannerVersionTunable, resolved2.Version)
	assert.EqualValues(t, 19, resolved2.PatchFactor)
	assert.EqualValues(t, 0, resolved2.PatchTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved2.CommitQueueFactor)
	assert.EqualValues(t, 0, resolved2.MainlineTimeInQueueFactor)
	assert.EqualValues(t, 0, resolved2.ExpectedRuntimeFactor)
	assert.EqualValues(t, 0, resolved2.GenerateTaskFactor)
}

func TestAddPermissions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(user.Collection, Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	env := evergreen.GetEnvironment()
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	u := user.DBUser{
		Id: "me",
	}
	require.NoError(t, u.Insert(t.Context()))
	d := Distro{
		Id: "myDistro",
	}
	require.NoError(t, d.Add(ctx, &u))

	rm := env.RoleManager()
	scope, err := rm.FindScopeForResources(t.Context(), evergreen.DistroResourceType, d.Id)
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	role, err := rm.FindRoleWithPermissions(t.Context(), evergreen.DistroResourceType, []string{d.Id}, map[string]int{
		evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value,
		evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
	})
	assert.NoError(t, err)
	assert.NotNil(t, role)
	dbUser, err := user.FindOneById(t.Context(), u.Id)
	assert.NoError(t, err)
	assert.Contains(t, dbUser.Roles(), "admin_distro_myDistro")
}

func TestLogDistroModifiedWithDistroData(t *testing.T) {
	assert.NoError(t, db.ClearCollections(event.EventCollection))

	oldDistro := Distro{
		Id:       "rainbow-lollipop",
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{
			birch.NewDocument().Set(birch.EC.String("ami", "ami-0")),
			birch.NewDocument().Set(birch.EC.SliceString("groups", []string{"group1", "group2"})),
		},
	}

	d := Distro{
		Id:       "rainbow-lollipop",
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{
			birch.NewDocument().Set(birch.EC.String("ami", "ami-123456")),
			birch.NewDocument().Set(birch.EC.SliceString("groups", []string{"group1", "group2"})),
		},
	}
	event.LogDistroModified(t.Context(), d.Id, "user1", oldDistro.DistroData(), d.DistroData())
	eventsForDistro, err := event.FindLatestPrimaryDistroEvents(t.Context(), d.Id, 10, utility.ZeroTime)
	assert.NoError(t, err)
	require.Len(t, eventsForDistro, 1)
	eventData, ok := eventsForDistro[0].Data.(*event.DistroEventData)
	assert.True(t, ok)
	assert.Equal(t, "user1", eventData.User)
	assert.NotNil(t, eventData.Before)
	assert.NotNil(t, eventData.After)

	// Test Before field
	data := DistroData{}
	body, err := bson.Marshal(eventData.Before)
	assert.NoError(t, err)
	assert.NoError(t, bson.Unmarshal(body, &data))
	require.NotNil(t, data)
	assert.Equal(t, oldDistro.Id, data.Distro.Id)
	assert.Equal(t, oldDistro.Provider, data.Distro.Provider)
	assert.Nil(t, data.Distro.ProviderSettingsList)
	require.Len(t, data.ProviderSettingsMap, 2)
	assert.EqualValues(t, oldDistro.ProviderSettingsList[0].ExportMap(), data.ProviderSettingsMap[0])
	assert.EqualValues(t, oldDistro.ProviderSettingsList[1].ExportMap(), data.ProviderSettingsMap[1])

	// Test After field
	data = DistroData{}
	body, err = bson.Marshal(eventData.After)
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
	env := &mock.Environment{Clients: evergreen.ClientConfig{S3URLPrefix: "https://foo.com"}}

	d := Distro{Arch: evergreen.ArchWindowsAmd64}
	assert.Equal(t, "https://foo.com/windows_amd64/evergreen.exe", d.S3ClientURL(env))

	d.Arch = evergreen.ArchLinuxAmd64
	assert.Equal(t, "https://foo.com/linux_amd64/evergreen", d.S3ClientURL(env))
}

func TestGetAuthorizedKeysFile(t *testing.T) {
	t.Run("ReturnsDistroAuthorizedKeysFile", func(t *testing.T) {
		expected := "/path/to/authorized_keys"
		d := Distro{
			User:               "user",
			Arch:               evergreen.ArchLinuxAmd64,
			AuthorizedKeysFile: expected,
		}
		assert.Equal(t, expected, d.GetAuthorizedKeysFile())
	})
	t.Run("DefaultsToDistroHomeDirectoryAuthorizedKeysFileIfDistroAuthorizedKeysIsUnset", func(t *testing.T) {
		expected := "/home/user/.ssh/authorized_keys"
		d := Distro{
			User: "user",
			Arch: evergreen.ArchLinuxAmd64,
		}
		assert.Equal(t, expected, d.GetAuthorizedKeysFile())
	})
}

func TestGetDistrosForImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(Collection))
	imageID := "distro"
	otherImageID := "not_distro"
	d1 := &Distro{
		Id:      "distro-1",
		ImageID: imageID,
	}
	assert.NoError(t, d1.Insert(ctx))
	d2 := &Distro{
		Id:      "distro-2",
		ImageID: imageID,
	}
	assert.NoError(t, d2.Insert(ctx))
	d3 := &Distro{
		Id:      "distro-3",
		ImageID: imageID,
	}
	assert.NoError(t, d3.Insert(ctx))
	d4 := &Distro{
		Id:      "distro-4",
		ImageID: otherImageID,
	}
	assert.NoError(t, d4.Insert(ctx))

	found, err := GetDistrosForImage(ctx, imageID)
	assert.NoError(t, err)
	assert.Len(t, found, 3)
}

func TestGetImageIDFromDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(Collection))
	d := &Distro{
		Id:      "ubuntu1804-large",
		ImageID: "ubuntu1804",
	}
	require.NoError(t, d.Insert(ctx))

	found, err := GetImageIDFromDistro(ctx, "ubuntu1804-large")
	require.NoError(t, err)
	assert.Equal(t, "ubuntu1804", found)
}

func TestCostData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.Clear(Collection))

	// Test distro with cost data
	d := &Distro{
		Id: "cost-test-distro",
		CostData: CostData{
			OnDemandRate:    0.25,
			SavingsPlanRate: 0.15,
		},
	}
	assert.NoError(t, d.Insert(ctx))

	found, err := FindOneId(ctx, d.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0.25, found.CostData.OnDemandRate)
	assert.Equal(t, 0.15, found.CostData.SavingsPlanRate)

	// Test without cost data (should default to zero values)
	d2 := &Distro{Id: "cost-test-distro-2"}
	assert.NoError(t, d2.Insert(ctx))

	found2, err := FindOneId(ctx, d2.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0.0, found2.CostData.OnDemandRate)
	assert.Equal(t, 0.0, found2.CostData.SavingsPlanRate)
}
