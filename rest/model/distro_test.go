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
