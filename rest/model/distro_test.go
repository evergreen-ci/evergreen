package model

import (
	"testing"

	"github.com/evergreen-ci/birch"
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
		HomeVolumeSettings: distro.HomeVolumeSettings{
			FormatCommand: "format_command",
		},
		IcecreamSettings: distro.IcecreamSettings{
			SchedulerHost: "host",
			ConfigPath:    "config_path",
		},
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
	assert.Equal(t, d.HomeVolumeSettings.FormatCommand, FromStringPtr(apiDistro.HomeVolumeSettings.FormatCommand))
	assert.Equal(t, d.IcecreamSettings.SchedulerHost, FromStringPtr(apiDistro.IcecreamSettings.SchedulerHost))
	assert.Equal(t, d.IcecreamSettings.ConfigPath, FromStringPtr(apiDistro.IcecreamSettings.ConfigPath))
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
			PreconditionScripts: []APIPreconditionScript{
				{
					Path:   ToStringPtr("/tmp/foo"),
					Script: ToStringPtr("echo foo"),
				},
			},
			FetchProvisioningScript: true,
		},
		Note: ToStringPtr("note1"),
		HomeVolumeSettings: APIHomeVolumeSettings{
			FormatCommand: ToStringPtr("format_command"),
		},
		IcecreamSettings: APIIcecreamSettings{
			SchedulerHost: ToStringPtr("host"),
			ConfigPath:    ToStringPtr("config_path"),
		},
	}

	res, err := apiDistro.ToService()
	require.NoError(t, err)

	d, ok := res.(*distro.Distro)
	require.True(t, ok)

	assert.Equal(t, FromStringPtr(apiDistro.CloneMethod), d.CloneMethod)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.Method), d.BootstrapSettings.Method)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.Communication), d.BootstrapSettings.Communication)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.ClientDir), d.BootstrapSettings.ClientDir)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.JasperBinaryDir), d.BootstrapSettings.JasperBinaryDir)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.JasperCredentialsPath), d.BootstrapSettings.JasperCredentialsPath)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.ServiceUser), d.BootstrapSettings.ServiceUser)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.ShellPath), d.BootstrapSettings.ShellPath)
	require.Len(t, d.BootstrapSettings.Env, 1)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.Env[0].Key), d.BootstrapSettings.Env[0].Key)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.Env[0].Value), d.BootstrapSettings.Env[0].Value)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumFiles, d.BootstrapSettings.ResourceLimits.NumFiles)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumProcesses, d.BootstrapSettings.ResourceLimits.NumProcesses)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.LockedMemoryKB, d.BootstrapSettings.ResourceLimits.LockedMemoryKB)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.VirtualMemoryKB, d.BootstrapSettings.ResourceLimits.VirtualMemoryKB)
	require.Len(t, d.BootstrapSettings.PreconditionScripts, 1)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.PreconditionScripts[0].Path), d.BootstrapSettings.PreconditionScripts[0].Path)
	assert.Equal(t, FromStringPtr(apiDistro.BootstrapSettings.PreconditionScripts[0].Script), d.BootstrapSettings.PreconditionScripts[0].Script)
	assert.True(t, apiDistro.BootstrapSettings.FetchProvisioningScript)
	assert.Equal(t, FromStringPtr(apiDistro.Note), (d.Note))
	assert.Equal(t, FromStringPtr(apiDistro.HomeVolumeSettings.FormatCommand), d.HomeVolumeSettings.FormatCommand)
	assert.Equal(t, FromStringPtr(apiDistro.IcecreamSettings.SchedulerHost), d.IcecreamSettings.SchedulerHost)
	assert.Equal(t, FromStringPtr(apiDistro.IcecreamSettings.ConfigPath), d.IcecreamSettings.ConfigPath)
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

func TestDistroAMIForEC2(t *testing.T) {
	settingsList := []*birch.Document{birch.NewDocument(birch.EC.String("ami", "ami-000000"))}
	d := distro.Distro{
		Id:                   "testId",
		Provider:             evergreen.ProviderNameEc2Auto,
		ProviderSettingsList: settingsList,
	}

	apiDistro := &APIDistro{}
	err := apiDistro.BuildFromService(d)
	assert.Nil(t, err)
	require.Len(t, apiDistro.ProviderSettingsList, 1)
	assert.Equal(t, "ami-000000", apiDistro.ProviderSettingsList[0].Lookup("ami").StringValue())
}
