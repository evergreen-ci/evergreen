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
		Note: "note1",
	}
	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	require.NoError(t, err)
	assert.Equal(t, FromStringPtr(apiDistro.Name), d.Id)
	assert.Equal(t, d.BootstrapSettings.Method, FromStringPtr(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, d.BootstrapSettings.Communication, FromStringPtr(apiDistro.BootstrapSettings.Communication))
	assert.Equal(t, d.BootstrapSettings.ClientDir, FromStringPtr(apiDistro.BootstrapSettings.ClientDir))
	assert.Equal(t, d.BootstrapSettings.JasperBinaryDir, FromStringPtr(apiDistro.BootstrapSettings.JasperBinaryDir))
	assert.Equal(t, d.BootstrapSettings.JasperCredentialsPath, FromStringPtr(apiDistro.BootstrapSettings.JasperCredentialsPath))
	assert.Equal(t, d.BootstrapSettings.ServiceUser, FromStringPtr(apiDistro.BootstrapSettings.ServiceUser))
	assert.Equal(t, d.BootstrapSettings.ShellPath, FromStringPtr(apiDistro.BootstrapSettings.ShellPath))
	assert.Equal(t, d.Note, FromStringPtr(apiDistro.Note))
}

func TestDistroBuildFromServiceDefaults(t *testing.T) {
	d := distro.Distro{
		Id: "id",
	}

	apiDistro := &APIDistro{}
	require.NoError(t, apiDistro.BuildFromService(d))

	assert.Equal(t, distro.BootstrapMethodLegacySSH, FromStringPtr(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, distro.CommunicationMethodLegacySSH, FromStringPtr(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, distro.CloneMethodLegacySSH, FromStringPtr(apiDistro.CloneMethod))
}

func TestDistroToService(t *testing.T) {
	apiDistro := APIDistro{
		Name:        ToStringPtr("id"),
		CloneMethod: ToStringPtr(distro.CloneMethodOAuth),
		BootstrapSettings: APIBootstrapSettings{
			Method:                ToStringPtr(distro.BootstrapMethodSSH),
			Communication:         ToStringPtr(distro.CommunicationMethodSSH),
			ClientDir:             ToStringPtr("/client_dir"),
			JasperBinaryDir:       ToStringPtr("/jasper_binary_dir"),
			JasperCredentialsPath: ToStringPtr("/jasper_credentials_path"),
			ServiceUser:           ToStringPtr("service_user"),
			ShellPath:             ToStringPtr("/shell_path"),
			Env:                   []APIEnvVar{{Key: ToStringPtr("envKey"), Value: ToStringPtr("envValue")}},
			ResourceLimits: APIResourceLimits{
				NumFiles:        1,
				NumProcesses:    2,
				LockedMemoryKB:  3,
				VirtualMemoryKB: 4,
			},
		},
		Note: ToStringPtr("note1"),
	}

	res, err := apiDistro.ToService()
	require.NoError(t, err)

	d, ok := res.(*distro.Distro)
	require.True(t, ok)

	assert.Equal(t, apiDistro.CloneMethod, ToStringPtr(d.CloneMethod))
	assert.Equal(t, apiDistro.BootstrapSettings.Method, ToStringPtr(d.BootstrapSettings.Method))
	assert.Equal(t, apiDistro.BootstrapSettings.Communication, ToStringPtr(d.BootstrapSettings.Communication))
	assert.Equal(t, apiDistro.BootstrapSettings.ClientDir, ToStringPtr(d.BootstrapSettings.ClientDir))
	assert.Equal(t, apiDistro.BootstrapSettings.JasperBinaryDir, ToStringPtr(d.BootstrapSettings.JasperBinaryDir))
	assert.Equal(t, apiDistro.BootstrapSettings.JasperCredentialsPath, ToStringPtr(d.BootstrapSettings.JasperCredentialsPath))
	assert.Equal(t, apiDistro.BootstrapSettings.ServiceUser, ToStringPtr(d.BootstrapSettings.ServiceUser))
	assert.Equal(t, apiDistro.BootstrapSettings.ShellPath, ToStringPtr(d.BootstrapSettings.ShellPath))
	assert.Equal(t, apiDistro.BootstrapSettings.Env, []APIEnvVar{{Key: ToStringPtr("envKey"), Value: ToStringPtr("envValue")}})
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumFiles, d.BootstrapSettings.ResourceLimits.NumFiles)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumProcesses, d.BootstrapSettings.ResourceLimits.NumProcesses)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.LockedMemoryKB, d.BootstrapSettings.ResourceLimits.LockedMemoryKB)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.VirtualMemoryKB, d.BootstrapSettings.ResourceLimits.VirtualMemoryKB)
	assert.Equal(t, apiDistro.Note, ToStringPtr(d.Note))
}

func TestDistroToServiceDefaults(t *testing.T) {
	apiDistro := APIDistro{
		Name: ToStringPtr("id"),
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
	assert.Equal(t, "ami-000000", FromStringPtr(apiDistro.ImageID))
}
