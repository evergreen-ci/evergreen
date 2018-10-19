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

func TestDistroNonSuperUserFilter(t *testing.T) {
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
		SSHOptions:   []string{"SSH Option1", "SSH Option2", "SSH Option3"},
		SpawnAllowed: true,
		Expansions: []distro.Expansion{
			distro.Expansion{Key: "key1", Value: "value1"},
			distro.Expansion{Key: "key2", Value: "value2"}},
		Disabled:      true,
		ContainerPool: "Container Pool",
	}

	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	assert.Nil(t, err)
	assert.Equal(t, []string{"SSH Option1", "SSH Option2", "SSH Option3"}, apiDistro.SSHOptions)
	apiDistro.Filter(false)
	assert.Nil(t, apiDistro.Expansions)
	assert.Nil(t, apiDistro.User)
	assert.Nil(t, apiDistro.Setup)
	assert.Nil(t, apiDistro.Teardown)
	assert.Nil(t, apiDistro.SSHKey)
}
