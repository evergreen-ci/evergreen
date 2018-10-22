package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
)

func TestDistroBuildFromService(t *testing.T) {
	d := distro.Distro{
		Id: "testId",
	}
	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	assert.Nil(t, err)
	assert.Equal(t, FromAPIString(apiDistro.Name), d.Id)
}

func TestDistroNoAMIForStatic(t *testing.T) {
	d := distro.Distro{
		Id:       "testId",
		Provider: "static",
	}

	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	assert.Nil(t, err)
	assert.Nil(t, apiDistro.ImageID)
}

func TestDistroAMIForEC2(t *testing.T) {
	d := distro.Distro{
		Id:       "testId",
		Provider: evergreen.ProviderNameEc2Auto,
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-000000",
		},
	}

	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	assert.Nil(t, err)
	assert.Equal(t, "ami-000000", FromAPIString(apiDistro.ImageID))
}

func TestDistroSuperUser(t *testing.T) {
	d := distro.Distro{
		Id:       "testId",
		Arch:     "Architecture",
		WorkDir:  "Work Directory",
		PoolSize: 10,
		Provider: evergreen.ProviderNameEc2Auto,
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-000000",
		},
		SetupAsSudo:  true,
		Setup:        "Setup Script",
		Teardown:     "Teardown Script",
		User:         "User Info",
		SSHKey:       "SSH Key",
		SSHOptions:   []string{"o1", "o2", "o3"},
		SpawnAllowed: true,
		Expansions: []distro.Expansion{
			distro.Expansion{Key: "k1", Value: "v1"},
			distro.Expansion{Key: "k2", Value: "v2"}},
		Disabled:      true,
		ContainerPool: "Container Pool",
	}

	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	assert.NoError(t, err)

	apiDistro.Filter(true)

	assert.Equal(t, "testId", FromAPIString(apiDistro.Name))
	assert.Equal(t, "Architecture", FromAPIString(apiDistro.Arch))
	assert.Equal(t, "Work Directory", FromAPIString(apiDistro.WorkDir))
	assert.Equal(t, 10, apiDistro.PoolSize)
	assert.Equal(t, true, apiDistro.SetupAsSudo)
	assert.Equal(t, "ami-000000", FromAPIString(apiDistro.ImageID))
	assert.Equal(t, "Setup Script", FromAPIString(apiDistro.Setup))
	assert.Equal(t, "Teardown Script", FromAPIString(apiDistro.Teardown))
	assert.Equal(t, "User Info", FromAPIString(apiDistro.User))
	assert.Equal(t, "SSH Key", FromAPIString(apiDistro.SSHKey))
	assert.Equal(t, []string{"o1", "o2", "o3"}, apiDistro.SSHOptions)

	assert.NotNil(t, apiDistro.Expansions)
	assert.Equal(t, "User Info", FromAPIString(apiDistro.User))
	assert.Equal(t, "Setup Script", FromAPIString(apiDistro.Setup))
	assert.Equal(t, "Teardown Script", FromAPIString(apiDistro.Teardown))
	assert.Equal(t, "SSH Key", FromAPIString(apiDistro.SSHKey))
}

func TestDistroNonSuperUser(t *testing.T) {
	d := distro.Distro{
		Id:       "testId",
		Arch:     "Architecture",
		WorkDir:  "Work Directory",
		PoolSize: 10,
		Provider: evergreen.ProviderNameEc2Auto,
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-000000",
		},
		SetupAsSudo:  true,
		Setup:        "Setup Script",
		Teardown:     "Teardown Script",
		User:         "User Info",
		SSHKey:       "SSH Key",
		SSHOptions:   []string{"o1", "o2", "o3"},
		SpawnAllowed: true,
		Expansions: []distro.Expansion{
			distro.Expansion{Key: "k1", Value: "v1"},
			distro.Expansion{Key: "k2", Value: "v2"}},
		Disabled:      true,
		ContainerPool: "Container Pool",
	}

	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	assert.NoError(t, err)

	apiDistro.Filter(false)

	assert.Equal(t, "testId", FromAPIString(apiDistro.Name))
	assert.Equal(t, "Architecture", FromAPIString(apiDistro.Arch))
	assert.Equal(t, "Work Directory", FromAPIString(apiDistro.WorkDir))
	assert.Equal(t, 10, apiDistro.PoolSize)
	assert.Equal(t, true, apiDistro.SetupAsSudo)
	assert.Equal(t, "ami-000000", FromAPIString(apiDistro.ImageID))
	assert.Equal(t, []string{"o1", "o2", "o3"}, apiDistro.SSHOptions)

	assert.Nil(t, apiDistro.Expansions)
	assert.Nil(t, apiDistro.User)
	assert.Nil(t, apiDistro.Setup)
	assert.Nil(t, apiDistro.Teardown)
	assert.Nil(t, apiDistro.SSHKey)
}
