package model

import (
	"testing"

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
