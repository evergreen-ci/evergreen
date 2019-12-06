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
		Id:          "testId",
		CloneMethod: distro.CloneMethodLegacySSH,
		BootstrapSettings: distro.BootstrapSettings{
			Method:                distro.BootstrapMethodUserData,
			ClientDir:             "/client_dir",
			Communication:         distro.CommunicationMethodRPC,
			JasperBinaryDir:       "/jasper_binary_dir",
			JasperCredentialsPath: "/jasper_credentials_path",
			ServiceUser:           "service_user",
			ShellPath:             "/shell_path",
		},
	}
	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	require.NoError(t, err)
	assert.Equal(t, FromAPIString(apiDistro.Name), d.Id)
	assert.Equal(t, d.BootstrapSettings.Method, FromAPIString(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, d.BootstrapSettings.Communication, FromAPIString(apiDistro.BootstrapSettings.Communication))
	assert.Equal(t, d.BootstrapSettings.ClientDir, FromAPIString(apiDistro.BootstrapSettings.ClientDir))
	assert.Equal(t, d.BootstrapSettings.JasperBinaryDir, FromAPIString(apiDistro.BootstrapSettings.JasperBinaryDir))
	assert.Equal(t, d.BootstrapSettings.JasperCredentialsPath, FromAPIString(apiDistro.BootstrapSettings.JasperCredentialsPath))
	assert.Equal(t, d.BootstrapSettings.ServiceUser, FromAPIString(apiDistro.BootstrapSettings.ServiceUser))
	assert.Equal(t, d.BootstrapSettings.ShellPath, FromAPIString(apiDistro.BootstrapSettings.ShellPath))
}

func TestDistroBuildFromServiceDefaults(t *testing.T) {
	d := distro.Distro{
		Id: "id",
	}

	apiDistro := &APIDistro{}
	require.NoError(t, apiDistro.BuildFromService(d))

	assert.Equal(t, distro.BootstrapMethodLegacySSH, FromAPIString(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, distro.CommunicationMethodLegacySSH, FromAPIString(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, distro.CloneMethodLegacySSH, FromAPIString(apiDistro.CloneMethod))
}

func TestDistroToService(t *testing.T) {
	apiDistro := APIDistro{
		Name:        ToAPIString("id"),
		CloneMethod: ToAPIString(distro.CloneMethodOAuth),
		BootstrapSettings: APIBootstrapSettings{
			Method:                ToAPIString(distro.BootstrapMethodSSH),
			Communication:         ToAPIString(distro.CommunicationMethodSSH),
			ClientDir:             ToAPIString("/client_dir"),
			JasperBinaryDir:       ToAPIString("/jasper_binary_dir"),
			JasperCredentialsPath: ToAPIString("/jasper_credentials_path"),
			ServiceUser:           ToAPIString("service_user"),
			ShellPath:             ToAPIString("/shell_path"),
			Env:                   []APIEnvVar{{Key: ToAPIString("envKey"), Value: ToAPIString("envValue")}},
			ResourceLimits: APIResourceLimits{
				NumFiles:        1,
				NumProcesses:    2,
				LockedMemoryKB:  3,
				VirtualMemoryKB: 4,
			},
		},
	}

	res, err := apiDistro.ToService()
	require.NoError(t, err)

	d, ok := res.(*distro.Distro)
	require.True(t, ok)

	assert.Equal(t, apiDistro.CloneMethod, ToAPIString(d.CloneMethod))
	assert.Equal(t, apiDistro.BootstrapSettings.Method, ToAPIString(d.BootstrapSettings.Method))
	assert.Equal(t, apiDistro.BootstrapSettings.Communication, ToAPIString(d.BootstrapSettings.Communication))
	assert.Equal(t, apiDistro.BootstrapSettings.ClientDir, ToAPIString(d.BootstrapSettings.ClientDir))
	assert.Equal(t, apiDistro.BootstrapSettings.JasperBinaryDir, ToAPIString(d.BootstrapSettings.JasperBinaryDir))
	assert.Equal(t, apiDistro.BootstrapSettings.JasperCredentialsPath, ToAPIString(d.BootstrapSettings.JasperCredentialsPath))
	assert.Equal(t, apiDistro.BootstrapSettings.ServiceUser, ToAPIString(d.BootstrapSettings.ServiceUser))
	assert.Equal(t, apiDistro.BootstrapSettings.ShellPath, ToAPIString(d.BootstrapSettings.ShellPath))
	assert.Equal(t, apiDistro.BootstrapSettings.Env, []APIEnvVar{{Key: ToAPIString("envKey"), Value: ToAPIString("envValue")}})
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumFiles, d.BootstrapSettings.ResourceLimits.NumFiles)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumProcesses, d.BootstrapSettings.ResourceLimits.NumProcesses)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.LockedMemoryKB, d.BootstrapSettings.ResourceLimits.LockedMemoryKB)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.VirtualMemoryKB, d.BootstrapSettings.ResourceLimits.VirtualMemoryKB)
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
	assert.Equal(t, distro.BootstrapMethodLegacySSH, d.BootstrapSettings.Method)
	assert.Equal(t, distro.CommunicationMethodLegacySSH, d.BootstrapSettings.Communication)
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
