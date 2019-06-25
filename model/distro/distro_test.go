package distro

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
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
	assert.True(d1.IsParent(nil))
	assert.False(d2.IsParent(settings))
	assert.False(d2.IsParent(nil))
	assert.False(d3.IsParent(settings))
	assert.False(d3.IsParent(nil))
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
	assert.Contains(err.Error(), "container pool test-pool-2 has invalid distro")
	assert.Contains(err.Error(), "error finding distro for container pool test-pool-3")
}

func TestGetDistroIds(t *testing.T) {
	assert := assert.New(t)
	hosts := DistroGroup{
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
	ids := hosts.GetDistroIds()
	assert.Equal([]string{"d1", "d2", "d3"}, ids)
}

func TestGetImageID(t *testing.T) {
	for _, test := range []struct {
		name           string
		provider       string
		key            string
		value          interface{}
		expectedOutput string
		err            bool
		noKey          bool
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
			name:     "InvalidType",
			provider: evergreen.ProviderNameEc2Auto,
			key:      "ami",
			value:    5,
			err:      true,
		},
		{
			name:     "InvalidKey",
			provider: evergreen.ProviderNameEc2Auto,
			key:      "abi",
			value:    "imageID",
			err:      true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			providerSettings := make(map[string]interface{})
			if !test.noKey {
				providerSettings[test.key] = test.value
			}
			distro := Distro{Provider: test.provider, ProviderSettings: &providerSettings}
			output, err := distro.GetImageID()
			assert.Equal(t, output, test.expectedOutput)
			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetResolvedPlannerSettings(t *testing.T) {
	d0 := Distro{
		Id: "distro0",
		PlannerSettings: PlannerSettings{
			Version:                "",
			MinimumHosts:           4,
			MaximumHosts:           10,
			TargetTime:             0,
			AcceptableHostIdleTime: 0,
			GroupVersions:          nil,
			PatchZipperFactor:      0,
			TaskOrdering:           "",
		},
	}
	config0 := evergreen.SchedulerConfig{
		TaskFinder:                    "legacy",
		HostAllocator:                 "legacy",
		FreeHostFraction:              0.1,
		CacheDurationSeconds:          60,
		Planner:                       evergreen.PlannerVersionLegacy,
		TargetTimeSeconds:             112358,
		AcceptableHostIdleTimeSeconds: 132134,
		GroupVersions:                 false,
		PatchZipperFactor:             50,
		TaskOrdering:                  evergreen.TaskOrderingInterleave,
	}

	resolved0, err := d0.GetResolvedPlannerSettings(config0)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.PlannerVersionLegacy, resolved0.Version)
	assert.Equal(t, 4, resolved0.MinimumHosts)
	assert.Equal(t, 10, resolved0.MaximumHosts)
	assert.Equal(t, time.Duration(112358)*time.Second, resolved0.TargetTime)
	assert.Equal(t, time.Duration(132134)*time.Second, resolved0.AcceptableHostIdleTime)
	// Fallback to the SchedulerConfig.GroupVersions as PlannerSettings.GroupVersions is nil
	assert.Equal(t, false, *resolved0.GroupVersions)
	assert.Equal(t, 50, resolved0.PatchZipperFactor)
	// Fallback to the SchedulerConfig task ordering as PlannerSettings.TaskOrdering is an empty string
	assert.Equal(t, evergreen.TaskOrderingInterleave, resolved0.TaskOrdering)

	pTrue := true
	d1 := Distro{
		Id: "distro1",
		PlannerSettings: PlannerSettings{
			Version:                evergreen.PlannerVersionTunable,
			MinimumHosts:           1,
			MaximumHosts:           5,
			TargetTime:             98765000000000,
			AcceptableHostIdleTime: 56789000000000,
			GroupVersions:          &pTrue,
			PatchZipperFactor:      25,
			TaskOrdering:           evergreen.TaskOrderingPatchFirst,
		},
	}
	config1 := evergreen.SchedulerConfig{
		TaskFinder:                    "legacy",
		HostAllocator:                 "legacy",
		FreeHostFraction:              0.1,
		CacheDurationSeconds:          60,
		Planner:                       evergreen.PlannerVersionLegacy, // STU: change this from PlannerVersion to Planner
		TargetTimeSeconds:             10,
		AcceptableHostIdleTimeSeconds: 60,
		GroupVersions:                 false,
		PatchZipperFactor:             50,
		TaskOrdering:                  evergreen.TaskOrderingInterleave,
	}

	// d1.PlannerSettings' field values are all set and valid, so there is no need to fallback on any SchedulerConfig field values
	resolved1, err := d1.GetResolvedPlannerSettings(config1)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.PlannerVersionTunable, resolved1.Version)
	assert.Equal(t, 1, resolved1.MinimumHosts)
	assert.Equal(t, 5, resolved1.MaximumHosts)
	assert.Equal(t, time.Duration(98765)*time.Second, resolved1.TargetTime)
	assert.Equal(t, time.Duration(56789)*time.Second, resolved1.AcceptableHostIdleTime)
	assert.Equal(t, true, *resolved1.GroupVersions)
	assert.Equal(t, 25, resolved1.PatchZipperFactor)
	assert.Equal(t, evergreen.TaskOrderingPatchFirst, resolved1.TaskOrdering)

	d2 := Distro{
		Id: "distro2",
		PlannerSettings: PlannerSettings{
			Version:                "",
			MinimumHosts:           7,
			MaximumHosts:           25,
			TargetTime:             0,
			AcceptableHostIdleTime: 0,
			GroupVersions:          nil,
			PatchZipperFactor:      25,
			TaskOrdering:           "",
		},
	}
	config2 := evergreen.SchedulerConfig{
		TaskFinder:                    "",
		HostAllocator:                 "",
		FreeHostFraction:              0.1,
		CacheDurationSeconds:          60,
		Planner:                       evergreen.PlannerVersionLegacy,
		TargetTimeSeconds:             12345,
		AcceptableHostIdleTimeSeconds: 67890,
		GroupVersions:                 false,
		PatchZipperFactor:             0,
		TaskOrdering:                  evergreen.TaskOrderingMainlineFirst,
	}
	resolved2, err := d2.GetResolvedPlannerSettings(config2)
	assert.NoError(t, err)
	// d2.PlannerSetting.Version is an empty string -- fallback on the SchedulerConfig.PlannerVersion value
	assert.Equal(t, evergreen.PlannerVersionLegacy, resolved2.Version)
	assert.Equal(t, 7, resolved2.MinimumHosts)
	assert.Equal(t, 25, resolved2.MaximumHosts)
	// d2.PlannerSetting.TargetTime and d2.PlannerSetting.AcceptableHostIdleTime are 0 -- fallback on the equivalent SchedulerConfig field vlaues
	assert.Equal(t, time.Duration(12345)*time.Second, resolved2.TargetTime)
	assert.Equal(t, time.Duration(67890)*time.Second, resolved2.AcceptableHostIdleTime)
	// d2.PlannerSetting.GroupVersions is nil -- fallback on the SchedulerConfig.PlannerVersion.GroupVersions value
	assert.Equal(t, false, *resolved2.GroupVersions)
	// d2.PlannerSetting.TaskOrdering is an empty string -- fallback on the SchedulerConfig.TaskOrdering value
	assert.Equal(t, evergreen.TaskOrderingMainlineFirst, resolved2.TaskOrdering)

	d2.PlannerSettings.Version = ""
	d2.PlannerSettings.MaximumHosts = -1
	config2.Planner = ""
	_, err = d2.GetResolvedPlannerSettings(config2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve PlannerSettings for distro 'distro2'")
	assert.Contains(t, err.Error(), "'' is not a valid PlannerSettings.Version")
	assert.Contains(t, err.Error(), "-1 is not a valid PlannerSettings.MaximumHosts")

	d2.PlannerSettings.Version = evergreen.PlannerVersionLegacy
	config2.TaskOrdering = ""
	_, err = d2.GetResolvedPlannerSettings(config2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve PlannerSettings for distro 'distro2'")
	assert.Contains(t, err.Error(), "'' is not a valid PlannerSettings.TaskOrdering")
}
