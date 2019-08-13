package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistroBuildFromService(t *testing.T) {
	d := distro.Distro{
		Id:                    "testId",
		CloneMethod:           distro.CloneMethodLegacySSH,
		BootstrapMethod:       distro.BootstrapMethodUserData,
		CommunicationMethod:   distro.CommunicationMethodRPC,
		CuratorDir:            "/curator_dir",
		JasperCredentialsPath: "/jasper_credentials_path",
		UserDataDonePath:      "/user_data_done",
	}
	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	require.NoError(t, err)
	assert.Equal(t, FromAPIString(apiDistro.Name), d.Id)
	assert.Equal(t, d.BootstrapMethod, FromAPIString(apiDistro.BootstrapMethod))
	assert.Equal(t, d.CommunicationMethod, FromAPIString(apiDistro.CommunicationMethod))
	assert.Equal(t, d.ShellPath, FromAPIString(apiDistro.ShellPath))
	assert.Equal(t, d.CuratorDir, FromAPIString(apiDistro.CuratorDir))
	assert.Equal(t, d.JasperCredentialsPath, FromAPIString(apiDistro.JasperCredentialsPath))
	assert.Equal(t, d.UserDataDonePath, FromAPIString(apiDistro.UserDataDonePath))
}

func TestDistroBuildFromServiceDefaults(t *testing.T) {
	d := distro.Distro{
		Id: "id",
	}

	apiDistro := &APIDistro{}
	require.NoError(t, apiDistro.BuildFromService(d))

	assert.Equal(t, distro.BootstrapMethodLegacySSH, FromAPIString(apiDistro.BootstrapMethod))
	assert.Equal(t, distro.CommunicationMethodLegacySSH, FromAPIString(apiDistro.CommunicationMethod))
	assert.Equal(t, distro.CloneMethodLegacySSH, FromAPIString(apiDistro.CloneMethod))
}

func TestDistroToService(t *testing.T) {
	apiDistro := APIDistro{
		Name:                  ToAPIString("id"),
		CloneMethod:           ToAPIString(distro.CloneMethodOAuth),
		BootstrapMethod:       ToAPIString(distro.BootstrapMethodSSH),
		CommunicationMethod:   ToAPIString(distro.CommunicationMethodSSH),
		ShellPath:             ToAPIString("/shell_path"),
		CuratorDir:            ToAPIString("/curator_dir"),
		JasperCredentialsPath: ToAPIString("/jasper_credentials_path"),
		UserDataDonePath:      ToAPIString("/user_data_done"),
	}

	res, err := apiDistro.ToService()
	require.NoError(t, err)

	d, ok := res.(*distro.Distro)
	require.True(t, ok)

	assert.Equal(t, apiDistro.CloneMethod, ToAPIString(d.CloneMethod))
	assert.Equal(t, apiDistro.BootstrapMethod, ToAPIString(d.BootstrapMethod))
	assert.Equal(t, apiDistro.CommunicationMethod, ToAPIString(d.CommunicationMethod))
	assert.Equal(t, apiDistro.ShellPath, ToAPIString(d.ShellPath))
	assert.Equal(t, apiDistro.CuratorDir, ToAPIString(d.CuratorDir))
	assert.Equal(t, apiDistro.JasperCredentialsPath, ToAPIString(d.JasperCredentialsPath))
	assert.Equal(t, apiDistro.UserDataDonePath, ToAPIString(d.UserDataDonePath))
}

func TestDistroToServiceDefaults(t *testing.T) {
	apiDistro := APIDistro{
		Name: ToAPIString("id"),
	}

	res, err := apiDistro.ToService()
	require.NoError(t, err)

	d, ok := res.(*distro.Distro)
	require.True(t, ok)

	assert.Equal(t, distro.CloneMethodLegacySSH, d.CloneMethod)
	assert.Equal(t, distro.BootstrapMethodLegacySSH, d.BootstrapMethod)
	assert.Equal(t, distro.CommunicationMethodLegacySSH, d.CommunicationMethod)
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
