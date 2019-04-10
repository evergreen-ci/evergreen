package distro

import (
	"regexp"
	"strings"
	"testing"

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

func TestJasperFileName(t *testing.T) {
	distro := Distro{Arch: ArchLinuxArm64}
	version := "abc123"
	expected := "curator-dist-linux-arm64-abc123.tar.gz"
	assert.Equal(t, expected, distro.JasperFileName(version))
}
