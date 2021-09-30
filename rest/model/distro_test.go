package model

import (
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/utility"
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
	assert.Equal(t, utility.FromStringPtr(apiDistro.Name), d.Id)
	assert.Equal(t, d.BootstrapSettings.Method, utility.FromStringPtr(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, d.BootstrapSettings.Communication, utility.FromStringPtr(apiDistro.BootstrapSettings.Communication))
	assert.Equal(t, d.BootstrapSettings.ClientDir, utility.FromStringPtr(apiDistro.BootstrapSettings.ClientDir))
	assert.Equal(t, d.BootstrapSettings.JasperBinaryDir, utility.FromStringPtr(apiDistro.BootstrapSettings.JasperBinaryDir))
	assert.Equal(t, d.BootstrapSettings.JasperCredentialsPath, utility.FromStringPtr(apiDistro.BootstrapSettings.JasperCredentialsPath))
	assert.Equal(t, d.BootstrapSettings.ServiceUser, utility.FromStringPtr(apiDistro.BootstrapSettings.ServiceUser))
	assert.Equal(t, d.BootstrapSettings.ShellPath, utility.FromStringPtr(apiDistro.BootstrapSettings.ShellPath))
	assert.Equal(t, d.Note, utility.FromStringPtr(apiDistro.Note))
	assert.Equal(t, d.HomeVolumeSettings.FormatCommand, utility.FromStringPtr(apiDistro.HomeVolumeSettings.FormatCommand))
	assert.Equal(t, d.IcecreamSettings.SchedulerHost, utility.FromStringPtr(apiDistro.IcecreamSettings.SchedulerHost))
	assert.Equal(t, d.IcecreamSettings.ConfigPath, utility.FromStringPtr(apiDistro.IcecreamSettings.ConfigPath))
}

func TestDistroBuildFromServiceDefaults(t *testing.T) {
	d := distro.Distro{
		Id: "id",
	}

	apiDistro := &APIDistro{}
	require.NoError(t, apiDistro.BuildFromService(d))

	assert.Equal(t, distro.BootstrapMethodLegacySSH, utility.FromStringPtr(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, distro.CommunicationMethodLegacySSH, utility.FromStringPtr(apiDistro.BootstrapSettings.Method))
	assert.Equal(t, distro.CloneMethodLegacySSH, utility.FromStringPtr(apiDistro.CloneMethod))
}

func TestDistroToService(t *testing.T) {
	apiDistro := APIDistro{
		Name:        utility.ToStringPtr("id"),
		CloneMethod: utility.ToStringPtr(distro.CloneMethodOAuth),
		BootstrapSettings: APIBootstrapSettings{
			Method:                utility.ToStringPtr(distro.BootstrapMethodSSH),
			Communication:         utility.ToStringPtr(distro.CommunicationMethodSSH),
			ClientDir:             utility.ToStringPtr("/client_dir"),
			JasperBinaryDir:       utility.ToStringPtr("/jasper_binary_dir"),
			JasperCredentialsPath: utility.ToStringPtr("/jasper_credentials_path"),
			ServiceUser:           utility.ToStringPtr("service_user"),
			ShellPath:             utility.ToStringPtr("/shell_path"),
			Env:                   []APIEnvVar{{Key: utility.ToStringPtr("envKey"), Value: utility.ToStringPtr("envValue")}},
			ResourceLimits: APIResourceLimits{
				NumFiles:        1,
				NumProcesses:    2,
				NumTasks:        3,
				LockedMemoryKB:  4,
				VirtualMemoryKB: 5,
			},
			PreconditionScripts: []APIPreconditionScript{
				{
					Path:   utility.ToStringPtr("/tmp/foo"),
					Script: utility.ToStringPtr("echo foo"),
				},
			},
		},
		Note: utility.ToStringPtr("note1"),
		HomeVolumeSettings: APIHomeVolumeSettings{
			FormatCommand: utility.ToStringPtr("format_command"),
		},
		IcecreamSettings: APIIcecreamSettings{
			SchedulerHost: utility.ToStringPtr("host"),
			ConfigPath:    utility.ToStringPtr("config_path"),
		},
	}

	res, err := apiDistro.ToService()
	require.NoError(t, err)

	d, ok := res.(*distro.Distro)
	require.True(t, ok)

	assert.Equal(t, utility.FromStringPtr(apiDistro.CloneMethod), d.CloneMethod)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.Method), d.BootstrapSettings.Method)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.Communication), d.BootstrapSettings.Communication)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.ClientDir), d.BootstrapSettings.ClientDir)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.JasperBinaryDir), d.BootstrapSettings.JasperBinaryDir)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.JasperCredentialsPath), d.BootstrapSettings.JasperCredentialsPath)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.ServiceUser), d.BootstrapSettings.ServiceUser)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.ShellPath), d.BootstrapSettings.ShellPath)
	require.Len(t, d.BootstrapSettings.Env, 1)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.Env[0].Key), d.BootstrapSettings.Env[0].Key)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.Env[0].Value), d.BootstrapSettings.Env[0].Value)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumFiles, d.BootstrapSettings.ResourceLimits.NumFiles)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumProcesses, d.BootstrapSettings.ResourceLimits.NumProcesses)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.NumTasks, d.BootstrapSettings.ResourceLimits.NumTasks)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.LockedMemoryKB, d.BootstrapSettings.ResourceLimits.LockedMemoryKB)
	assert.Equal(t, apiDistro.BootstrapSettings.ResourceLimits.VirtualMemoryKB, d.BootstrapSettings.ResourceLimits.VirtualMemoryKB)
	require.Len(t, d.BootstrapSettings.PreconditionScripts, 1)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.PreconditionScripts[0].Path), d.BootstrapSettings.PreconditionScripts[0].Path)
	assert.Equal(t, utility.FromStringPtr(apiDistro.BootstrapSettings.PreconditionScripts[0].Script), d.BootstrapSettings.PreconditionScripts[0].Script)
	assert.Equal(t, utility.FromStringPtr(apiDistro.Note), (d.Note))
	assert.Equal(t, utility.FromStringPtr(apiDistro.HomeVolumeSettings.FormatCommand), d.HomeVolumeSettings.FormatCommand)
	assert.Equal(t, utility.FromStringPtr(apiDistro.IcecreamSettings.SchedulerHost), d.IcecreamSettings.SchedulerHost)
	assert.Equal(t, utility.FromStringPtr(apiDistro.IcecreamSettings.ConfigPath), d.IcecreamSettings.ConfigPath)
}

func TestDistroToServiceDefaults(t *testing.T) {
	apiDistro := APIDistro{
		Name: utility.ToStringPtr("id"),
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
