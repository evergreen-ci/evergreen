package cocoa

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECSPodCreationOptions(t *testing.T) {
	t.Run("NewECSPodCreationOptions", func(t *testing.T) {
		opts := NewECSPodCreationOptions()
		require.NotZero(t, opts)
		assert.Zero(t, *opts)
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		def := NewECSPodCreationOptions().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(def.Name))
	})
	t.Run("SetContainerDefinitions", func(t *testing.T) {
		containerDef := NewECSContainerDefinition().SetImage("image")

		opts := NewECSPodCreationOptions().SetContainerDefinitions([]ECSContainerDefinition{*containerDef})
		require.Len(t, opts.ContainerDefinitions, 1)
		assert.Equal(t, *containerDef, opts.ContainerDefinitions[0])

		opts.SetContainerDefinitions(nil)
		assert.Empty(t, opts.ContainerDefinitions)
	})
	t.Run("AddContainerDefinitions", func(t *testing.T) {
		containerDef0 := NewECSContainerDefinition().SetImage("image0")
		containerDef1 := NewECSContainerDefinition().SetImage("image1")
		def := NewECSPodCreationOptions().AddContainerDefinitions(*containerDef0, *containerDef1)
		require.Len(t, def.ContainerDefinitions, 2)
		assert.Equal(t, *containerDef0, def.ContainerDefinitions[0])
		assert.Equal(t, *containerDef1, def.ContainerDefinitions[1])
		def.AddContainerDefinitions()
		require.Len(t, def.ContainerDefinitions, 2)
		assert.Equal(t, *containerDef0, def.ContainerDefinitions[0])
		assert.Equal(t, *containerDef1, def.ContainerDefinitions[1])
	})
	t.Run("SetMemoryMB", func(t *testing.T) {
		mem := 128
		opts := NewECSPodCreationOptions().SetMemoryMB(mem)
		assert.Equal(t, mem, utility.FromIntPtr(opts.MemoryMB))
	})
	t.Run("SetCPU", func(t *testing.T) {
		cpu := 128
		opts := NewECSPodCreationOptions().SetCPU(cpu)
		assert.Equal(t, cpu, utility.FromIntPtr(opts.CPU))
	})
	t.Run("SetNetworkMode", func(t *testing.T) {
		mode := NetworkModeAWSVPC
		opts := NewECSPodCreationOptions().SetNetworkMode(mode)
		require.NotZero(t, opts.NetworkMode)
		assert.Equal(t, mode, *opts.NetworkMode)
	})
	t.Run("SetTaskRole", func(t *testing.T) {
		r := "task_role"
		opts := NewECSPodCreationOptions().SetTaskRole(r)
		assert.Equal(t, r, utility.FromStringPtr(opts.TaskRole))
	})
	t.Run("SetExecutionRole", func(t *testing.T) {
		r := "execution_role"
		opts := NewECSPodCreationOptions().SetExecutionRole(r)
		assert.Equal(t, r, utility.FromStringPtr(opts.ExecutionRole))
	})
	t.Run("SetTags", func(t *testing.T) {
		tags := map[string]string{"key": "value"}

		opts := NewECSPodCreationOptions().SetTags(tags)
		require.Len(t, opts.Tags, len(tags))
		for k, v := range tags {
			assert.Equal(t, v, opts.Tags[k])
		}

		opts.SetTags(nil)
		assert.Empty(t, opts.Tags)
	})
	t.Run("AddTags", func(t *testing.T) {
		tags := map[string]string{"key0": "val0", "key1": "val1"}
		opts := NewECSPodCreationOptions().AddTags(tags)
		require.Len(t, opts.Tags, len(tags))
		for k, v := range tags {
			assert.Equal(t, v, opts.Tags[k])
		}
		opts.AddTags(map[string]string{})
		assert.Len(t, opts.Tags, len(tags))
		for k, v := range tags {
			assert.Equal(t, v, opts.Tags[k])
		}
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			assert.Error(t, NewECSPodCreationOptions().Validate())
		})
		t.Run("MemoryCPUAndContainerDefinitionIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128)
			assert.NoError(t, opts.Validate())
		})
		t.Run("MissingContainerDefinitionsIsInvalid", func(t *testing.T) {
			opts := NewECSPodCreationOptions().
				SetMemoryMB(128).
				SetCPU(128)
			assert.Error(t, opts.Validate())
		})
		t.Run("NameIsGenerated", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128)
			assert.NoError(t, opts.Validate())
			assert.NotZero(t, utility.FromStringPtr(opts.Name))
		})
		t.Run("BadContainerDefinitionIsInvalid", func(t *testing.T) {
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*NewECSContainerDefinition()).
				SetMemoryMB(128).
				SetCPU(128)
			assert.Error(t, opts.Validate())
		})
		t.Run("AllFieldsPopulatedIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				SetName("name").
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetTaskRole("role").
				AddTags(map[string]string{"key": "val"}).
				SetExecutionOptions(*NewECSPodExecutionOptions())
			assert.NoError(t, opts.Validate())
		})
		t.Run("MissingCPUIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128)
			assert.Error(t, opts.Validate())
		})
		t.Run("MissingPodCPUWithContainerCPUIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetCPU(128)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128)
			assert.NoError(t, opts.Validate())
		})
		t.Run("TotalContainerCPUCannotExceedPodCPU", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetCPU(256)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(1024).
				SetCPU(128)
			assert.Error(t, opts.Validate())
		})
		t.Run("ZeroCPUIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(0)
			assert.Error(t, opts.Validate())
		})
		t.Run("MissingMemoryIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetCPU(128)
			assert.Error(t, opts.Validate())
		})
		t.Run("MissingPodMemoryWithContainerMemoryIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetMemoryMB(128)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetCPU(128)
			assert.NoError(t, opts.Validate())
		})
		t.Run("TotalContainerMemoryCannotExceedPodMemory", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetMemoryMB(256)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(1024)
			assert.Error(t, opts.Validate())
		})
		t.Run("ZeroMemoryIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(0).
				SetCPU(128)
			assert.Error(t, opts.Validate())
		})
		t.Run("BadExecutionOptionsIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			placementOpts := NewECSPodPlacementOptions().SetStrategy("foo")
			execOpts := NewECSPodExecutionOptions().SetPlacementOptions(*placementOpts)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetExecutionOptions(*execOpts)
			assert.Error(t, opts.Validate())
		})
		t.Run("SecretEnvironmentVariablesWithoutExecutionRoleIsInvalid", func(t *testing.T) {
			secretOpts := NewSecretOptions().SetName("name").SetValue("value")
			ev := NewEnvironmentVariable().SetName("name").SetSecretOptions(*secretOpts)
			containerDef := NewECSContainerDefinition().SetImage("image").AddEnvironmentVariables(*ev)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128)
			assert.Error(t, opts.Validate())
		})
		t.Run("BadNetworkModeIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode("invalid")
			assert.Error(t, opts.Validate())
		})
		t.Run("NoPortMappingsWithNetworkModeNoneIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeNone)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingsWithNetworkModeNoneIsInvalid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeNone)
			assert.Error(t, opts.Validate())
		})
		t.Run("PortMappingToIdenticalPortWithNetworkModeAWSVPCIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337).SetHostPort(1337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeAWSVPC)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingToUnspecifiedHostPortWithNetworkModeAWSVPCIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeAWSVPC)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingsToMismatchedPortWithNetworkModeAWSVPCIsInvalid", func(t *testing.T) {
			pm := NewPortMapping().
				SetContainerPort(1337).
				SetHostPort(13337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeAWSVPC)
			assert.Error(t, opts.Validate())
		})
		t.Run("PortMappingToIdenticalPortWithNetworkModeHostIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337).SetHostPort(1337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeHost)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingToUnspecifiedHostPortWithNetworkModeHostIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeHost)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingsToMismatchedPortWithNetworkModeHostIsInvalid", func(t *testing.T) {
			pm := NewPortMapping().
				SetContainerPort(1337).
				SetHostPort(13337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeBridge)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingToIdenticalPortWithNetworkModeBridgeIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337).SetHostPort(1337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeHost)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingToUnspecifiedHostPortWithNetworkModeBridgeIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeBridge)
			assert.NoError(t, opts.Validate())
		})
		t.Run("PortMappingsToMismatchedPortWithNetworkModeBridgeIsValid", func(t *testing.T) {
			pm := NewPortMapping().
				SetContainerPort(1337).
				SetHostPort(13337)
			containerDef := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*pm)
			opts := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(NetworkModeBridge)
			assert.NoError(t, opts.Validate())
		})
	})
}

func TestECSNetworkMode(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		for _, m := range []ECSNetworkMode{
			NetworkModeNone,
			NetworkModeAWSVPC,
			NetworkModeBridge,
			NetworkModeHost,
		} {
			t.Run(fmt.Sprintf("SucceedsForStatus=%s", m), func(t *testing.T) {
				assert.NoError(t, m.Validate())
			})
		}
		t.Run("FailsForEmptyStatus", func(t *testing.T) {
			assert.Error(t, ECSNetworkMode("").Validate())
		})
		t.Run("FailsForInvalidStatus", func(t *testing.T) {
			assert.Error(t, ECSNetworkMode("invalid").Validate())
		})
	})
}

func TestECSContainerDefinition(t *testing.T) {
	t.Run("NewECSContainerDefinition", func(t *testing.T) {
		def := NewECSContainerDefinition()
		require.NotZero(t, def)
		assert.Zero(t, *def)
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		def := NewECSContainerDefinition().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(def.Name))
	})
	t.Run("SetImage", func(t *testing.T) {
		image := "image"
		def := NewECSContainerDefinition().SetImage(image)
		assert.Equal(t, image, utility.FromStringPtr(def.Image))
	})
	t.Run("SetCommand", func(t *testing.T) {
		cmd := []string{"echo", "hello"}
		def := NewECSContainerDefinition().SetCommand(cmd)
		assert.Equal(t, cmd, def.Command)
	})
	t.Run("SetWorkingDir", func(t *testing.T) {
		dir := "working_dir"
		def := NewECSContainerDefinition().SetWorkingDir(dir)
		assert.Equal(t, dir, utility.FromStringPtr(def.WorkingDir))
	})
	t.Run("SetMemoryMB", func(t *testing.T) {
		mem := 128
		def := NewECSContainerDefinition().SetMemoryMB(mem)
		assert.Equal(t, mem, utility.FromIntPtr(def.MemoryMB))
	})
	t.Run("SetCPU", func(t *testing.T) {
		cpu := 128
		def := NewECSContainerDefinition().SetCPU(cpu)
		assert.Equal(t, cpu, utility.FromIntPtr(def.CPU))
	})
	t.Run("SetEnvironmentVariables", func(t *testing.T) {
		ev := NewEnvironmentVariable().SetName("name").SetValue("value")

		def := NewECSContainerDefinition().SetEnvironmentVariables([]EnvironmentVariable{*ev})
		require.Len(t, def.EnvVars, 1)
		assert.Equal(t, *ev, def.EnvVars[0])

		def.SetEnvironmentVariables(nil)
		assert.Empty(t, def.EnvVars)
	})
	t.Run("AddEnvironmentVariables", func(t *testing.T) {
		ev0 := NewEnvironmentVariable().SetName("name0").SetValue("value0")
		ev1 := NewEnvironmentVariable().SetName("name1").SetValue("value1")
		def := NewECSContainerDefinition().AddEnvironmentVariables(*ev0, *ev1)
		require.Len(t, def.EnvVars, 2)
		assert.Equal(t, *ev0, def.EnvVars[0])
		assert.Equal(t, *ev1, def.EnvVars[1])
		def.AddEnvironmentVariables()
		require.Len(t, def.EnvVars, 2)
		assert.Equal(t, *ev0, def.EnvVars[0])
		assert.Equal(t, *ev1, def.EnvVars[1])
	})
	t.Run("SetRepositoryCredentials", func(t *testing.T) {
		creds := NewRepositoryCredentials().SetSecretName("name")
		def := NewECSContainerDefinition().SetRepositoryCredentials(*creds)
		require.NotZero(t, def.RepoCreds)
		assert.Equal(t, utility.FromStringPtr(creds.SecretName), utility.FromStringPtr(def.RepoCreds.SecretName))
	})
	t.Run("SetPortMappings", func(t *testing.T) {
		pm := NewPortMapping().SetContainerPort(1337)

		def := NewECSContainerDefinition().SetPortMappings([]PortMapping{*pm})
		require.Len(t, def.PortMappings, 1)
		assert.Equal(t, *pm, def.PortMappings[0])

		def = NewECSContainerDefinition().SetPortMappings(nil)
		assert.Empty(t, def.PortMappings)
	})
	t.Run("AddPortMappings", func(t *testing.T) {
		pm0 := NewPortMapping().SetContainerPort(1337)
		pm1 := NewPortMapping().SetContainerPort(23456)

		def := NewECSContainerDefinition().AddPortMappings(*pm0, *pm1)
		require.Len(t, def.PortMappings, 2)
		assert.Equal(t, *pm0, def.PortMappings[0])
		assert.Equal(t, *pm1, def.PortMappings[1])

		def.AddPortMappings()
		require.Len(t, def.PortMappings, 2)
		assert.Equal(t, *pm0, def.PortMappings[0])
		assert.Equal(t, *pm1, def.PortMappings[1])
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			assert.Error(t, NewECSContainerDefinition().Validate())
		})
		t.Run("OnlyImageIsValid", func(t *testing.T) {
			def := NewECSContainerDefinition().SetImage("image")
			assert.NoError(t, def.Validate())
		})
		t.Run("MissingImageIsInvalid", func(t *testing.T) {
			def := NewECSContainerDefinition().
				SetMemoryMB(128).
				SetCPU(128)
			assert.Error(t, def.Validate())
		})
		t.Run("NameIsGenerated", func(t *testing.T) {
			def := NewECSContainerDefinition().SetImage("image")
			assert.NoError(t, def.Validate())
			assert.NotZero(t, utility.FromStringPtr(def.Name))
		})
		t.Run("AllFieldsPopulatedIsValid", func(t *testing.T) {
			ev := NewEnvironmentVariable().SetName("name").SetValue("value")
			def := NewECSContainerDefinition().
				SetImage("image").
				SetMemoryMB(128).
				SetCPU(128).
				SetCommand([]string{"echo"}).
				AddEnvironmentVariables(*ev)
			assert.NoError(t, def.Validate())
		})
		t.Run("ZeroCPUIsInvalid", func(t *testing.T) {
			def := NewECSContainerDefinition().
				SetImage("image").
				SetCPU(0)
			assert.Error(t, def.Validate())
		})
		t.Run("ZeroMemoryIsInvalid", func(t *testing.T) {
			def := NewECSContainerDefinition().
				SetImage("image").
				SetMemoryMB(0)
			assert.Error(t, def.Validate())
		})
		t.Run("BadEnvironmentVariablesIsInvalid", func(t *testing.T) {
			def := NewECSContainerDefinition().
				SetImage("image").
				AddEnvironmentVariables(*NewEnvironmentVariable())
			assert.Error(t, def.Validate())
		})
		t.Run("BadRepositoryCredentialsIsInvalid", func(t *testing.T) {
			def := NewECSContainerDefinition().
				SetImage("image").
				SetRepositoryCredentials(*NewRepositoryCredentials())
			assert.Error(t, def.Validate())
		})
		t.Run("BadPortMappingIsInvalid", func(t *testing.T) {
			def := NewECSContainerDefinition().
				SetImage("image").
				AddPortMappings(*NewPortMapping())
			assert.Error(t, def.Validate())
		})
	})
}

func TestEnvironmentVariable(t *testing.T) {
	t.Run("NewEnvironmentVariable", func(t *testing.T) {
		ev := NewEnvironmentVariable()
		require.NotZero(t, ev)
		assert.Zero(t, *ev)
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		ev := NewEnvironmentVariable().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(ev.Name))
	})
	t.Run("SetValue", func(t *testing.T) {
		val := "value"
		ev := NewEnvironmentVariable().SetValue(val)
		assert.Equal(t, val, utility.FromStringPtr(ev.Value))
	})
	t.Run("SetSecretOptions", func(t *testing.T) {
		opts := NewSecretOptions().SetName("name").SetValue("value")
		ev := NewEnvironmentVariable().SetSecretOptions(*opts)
		require.NotNil(t, ev.SecretOpts)
		assert.Equal(t, utility.FromStringPtr(opts.Name), utility.FromStringPtr(ev.SecretOpts.Name))
		assert.Equal(t, utility.FromStringPtr(opts.Value), utility.FromStringPtr(ev.SecretOpts.Value))
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			var ev EnvironmentVariable
			assert.Error(t, ev.Validate())
		})
		t.Run("NameAndValueIsValid", func(t *testing.T) {
			ev := EnvironmentVariable{
				Name:  utility.ToStringPtr("name"),
				Value: utility.ToStringPtr("value"),
			}
			assert.NoError(t, ev.Validate())
		})
		t.Run("NameAndSecretIsValid", func(t *testing.T) {
			ev := EnvironmentVariable{
				Name:       utility.ToStringPtr("name"),
				SecretOpts: NewSecretOptions().SetName("secret_name").SetValue("secret_value"),
			}
			assert.NoError(t, ev.Validate())
		})
		t.Run("NameAndInvalidSecretIsInvalid", func(t *testing.T) {
			ev := EnvironmentVariable{
				Name:       utility.ToStringPtr("name"),
				SecretOpts: NewSecretOptions(),
			}
			assert.Error(t, ev.Validate())
		})
		t.Run("MissingNameIsInvalid", func(t *testing.T) {
			ev := EnvironmentVariable{
				Value: utility.ToStringPtr("value"),
			}
			assert.Error(t, ev.Validate())
		})
		t.Run("EmptyNameIsInvalid", func(t *testing.T) {
			ev := EnvironmentVariable{
				Name:  utility.ToStringPtr(""),
				Value: utility.ToStringPtr("value"),
			}
			assert.Error(t, ev.Validate())
		})
		t.Run("SettingValueAndSecretIsInvalid", func(t *testing.T) {
			ev := EnvironmentVariable{
				Name:       utility.ToStringPtr("name"),
				Value:      utility.ToStringPtr("value"),
				SecretOpts: NewSecretOptions().SetName("secret_name").SetValue("secret_value"),
			}
			assert.Error(t, ev.Validate())
		})
		t.Run("MissingValueAndSecretIsInvalid", func(t *testing.T) {
			ev := EnvironmentVariable{
				Name: utility.ToStringPtr("name"),
			}
			assert.Error(t, ev.Validate())
		})
	})
}

func TestRepositoryCredentials(t *testing.T) {
	t.Run("NewRepositoryCredentials", func(t *testing.T) {
		creds := NewRepositoryCredentials()
		require.NotZero(t, creds)
		assert.Zero(t, *creds)
	})
	t.Run("SetSecretName", func(t *testing.T) {
		name := "secret_name"
		creds := NewRepositoryCredentials().SetSecretName(name)
		assert.Equal(t, name, utility.FromStringPtr(creds.SecretName))
	})
	t.Run("SetOwned", func(t *testing.T) {
		creds := NewRepositoryCredentials().SetOwned(true)
		assert.True(t, utility.FromBoolPtr(creds.Owned))
	})
	t.Run("SetNewCredentials", func(t *testing.T) {
		storedCreds := NewStoredRepositoryCredentials().
			SetUsername("username").
			SetPassword("password")
		creds := NewRepositoryCredentials().SetNewCredentials(*storedCreds)
		require.NotZero(t, creds.NewCreds)
		assert.Equal(t, *storedCreds, *creds.NewCreds)
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("SucceedsIfNewCredsAreSet", func(t *testing.T) {
			storedCreds := NewStoredRepositoryCredentials().
				SetUsername("username").
				SetPassword("password")
			creds := NewRepositoryCredentials().
				SetSecretName("name").
				SetNewCredentials(*storedCreds)
			assert.NoError(t, creds.Validate())
		})
		t.Run("SucceedsWithJustSecretNameForExistingSecret", func(t *testing.T) {
			creds := NewRepositoryCredentials().SetSecretName("name")
			assert.NoError(t, creds.Validate())
		})
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			creds := NewRepositoryCredentials()
			assert.Error(t, creds.Validate())
		})
		t.Run("MissingSecretNameIsInvalid", func(t *testing.T) {
			storedCreds := NewStoredRepositoryCredentials().
				SetUsername("username").
				SetPassword("password")
			creds := NewRepositoryCredentials().SetNewCredentials(*storedCreds)
			assert.Error(t, creds.Validate())
		})
		t.Run("BadNewCredentialsIsInvalid", func(t *testing.T) {
			storedCreds := NewStoredRepositoryCredentials()
			creds := NewRepositoryCredentials().SetNewCredentials(*storedCreds)
			assert.Error(t, creds.Validate())
		})
	})
}

func TestStoredRepositoryCredentials(t *testing.T) {
	t.Run("SetUsername", func(t *testing.T) {
		username := "username"
		creds := NewStoredRepositoryCredentials().SetUsername(username)
		assert.Equal(t, username, utility.FromStringPtr(creds.Username))
	})
	t.Run("SetPassword", func(t *testing.T) {
		password := "password"
		creds := NewStoredRepositoryCredentials().SetPassword(password)
		assert.Equal(t, password, utility.FromStringPtr(creds.Password))
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("SucceedsWithUsernameAndPassword", func(t *testing.T) {
			creds := NewStoredRepositoryCredentials().
				SetUsername("username").
				SetPassword("password")
			assert.NoError(t, creds.Validate())
		})
		t.Run("FailsWithoutUsername", func(t *testing.T) {
			creds := NewStoredRepositoryCredentials().SetPassword("password")
			assert.Error(t, creds.Validate())
		})
		t.Run("FailsWithoutPassword", func(t *testing.T) {
			creds := NewStoredRepositoryCredentials().SetPassword("password")
			assert.Error(t, creds.Validate())
		})
	})
}

func TestSecretOptions(t *testing.T) {
	t.Run("NewSecretOptions", func(t *testing.T) {
		opts := NewSecretOptions()
		require.NotZero(t, opts)
		assert.Zero(t, *opts)
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		opts := NewSecretOptions().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(opts.Name))
	})
	t.Run("SetValue", func(t *testing.T) {
		val := "value"
		opts := NewSecretOptions().SetValue(val)
		assert.Equal(t, val, utility.FromStringPtr(opts.Value))
	})
	t.Run("SetExists", func(t *testing.T) {
		opts := NewSecretOptions().SetExists(true)
		assert.True(t, utility.FromBoolPtr(opts.Exists))
	})
	t.Run("SetOwned", func(t *testing.T) {
		opts := NewSecretOptions().SetOwned(true)
		assert.True(t, utility.FromBoolPtr(opts.Owned))
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("NameAndNewValueIsValid", func(t *testing.T) {
			s := NewSecretOptions().SetName("name").SetValue("value")
			assert.NoError(t, s.Validate())
		})
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			s := NewSecretOptions()
			assert.Error(t, s.Validate())
		})
		t.Run("ExistingSecretWithNameIsValid", func(t *testing.T) {
			s := NewSecretOptions().SetName("name").SetExists(true)
			assert.NoError(t, s.Validate())
		})
		t.Run("MissingNameIsInvalid", func(t *testing.T) {
			s := NewSecretOptions().SetValue("value")
			assert.Error(t, s.Validate())
		})
		t.Run("ExistingSecretWithNewValueIsInvalid", func(t *testing.T) {
			s := NewSecretOptions().SetName("name").SetExists(true).SetValue("value")
			assert.Error(t, s.Validate())
		})
	})
}

func TestPortMappings(t *testing.T) {
	t.Run("NewPortMapping", func(t *testing.T) {
		pm := NewPortMapping()
		require.NotZero(t, pm)
		assert.Zero(t, *pm)
	})
	t.Run("SetContainerPort", func(t *testing.T) {
		port := 1337
		pm := NewPortMapping().SetContainerPort(1337)
		assert.Equal(t, port, utility.FromIntPtr(pm.ContainerPort))
	})
	t.Run("SetHostPort", func(t *testing.T) {
		port := 1337
		pm := NewPortMapping().SetHostPort(1337)
		assert.Equal(t, port, utility.FromIntPtr(pm.HostPort))
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			pm := NewPortMapping()
			assert.Error(t, pm.Validate())
		})
		t.Run("JustContainerPortIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337)
			assert.NoError(t, pm.Validate())
		})
		t.Run("ContainerAndHostPortIsValid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(1337).SetHostPort(1337)
			assert.NoError(t, pm.Validate())
		})
		t.Run("NegativeContainerPortIsInvalid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(-100)
			assert.Error(t, pm.Validate())
		})
		t.Run("ContainerPortAboveMaxIsInvalid", func(t *testing.T) {
			pm := NewPortMapping().SetContainerPort(100000)
			assert.Error(t, pm.Validate())
		})
		t.Run("NegativeHostPortIsInvalid", func(t *testing.T) {
			pm := NewPortMapping().
				SetContainerPort(1337).
				SetHostPort(-100)
			assert.Error(t, pm.Validate())
		})
		t.Run("HostPortAboveMaxIsInvalid", func(t *testing.T) {
			pm := NewPortMapping().
				SetContainerPort(1337).
				SetHostPort(100000)
			assert.Error(t, pm.Validate())
		})
	})
}

func TestECSPodExecutionOptions(t *testing.T) {
	t.Run("NewECSPodExecutionOptions", func(t *testing.T) {
		opts := NewECSPodExecutionOptions()
		require.NotZero(t, opts)
		assert.Zero(t, *opts)
	})
	t.Run("SetCluster", func(t *testing.T) {
		cluster := "cluster"
		opts := NewECSPodExecutionOptions().SetCluster("cluster")
		assert.Equal(t, cluster, utility.FromStringPtr(opts.Cluster))
	})
	t.Run("SetPlacementOptions", func(t *testing.T) {
		placementOpts := NewECSPodPlacementOptions().SetStrategy(StrategyBinpack)
		opts := NewECSPodExecutionOptions().SetPlacementOptions(*placementOpts)
		require.NotZero(t, opts.PlacementOpts)
		assert.Equal(t, *placementOpts, *opts.PlacementOpts)
	})
	t.Run("SetSupportsDebugMode", func(t *testing.T) {
		opts := NewECSPodExecutionOptions().SetSupportsDebugMode(true)
		assert.True(t, utility.FromBoolPtr(opts.SupportsDebugMode))
	})
	t.Run("SetTags", func(t *testing.T) {
		key := "tkey"
		value := "tvalue"
		opts := NewECSPodExecutionOptions().SetTags(map[string]string{key: value})
		require.Len(t, opts.Tags, 1)
		assert.Equal(t, value, opts.Tags[key])
	})
	t.Run("AddTags", func(t *testing.T) {
		key0 := "key0"
		val0 := "val0"
		key1 := "key1"
		val1 := "val1"
		opts := NewECSPodExecutionOptions().AddTags(map[string]string{key0: val0, key1: val1})
		require.Len(t, opts.Tags, 2)
		assert.Equal(t, val0, opts.Tags[key0])
		assert.Equal(t, val1, opts.Tags[key1])
		opts.AddTags(map[string]string{})
		assert.Len(t, opts.Tags, 2)
		assert.Equal(t, val0, opts.Tags[key0])
		assert.Equal(t, val1, opts.Tags[key1])
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsValid", func(t *testing.T) {
			opts := NewECSPodExecutionOptions()
			assert.NoError(t, opts.Validate())
		})
		t.Run("NoPlacementOptionsAreDefaultedToBinpackMemory", func(t *testing.T) {
			opts := NewECSPodExecutionOptions()
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.PlacementOpts)
			require.NotZero(t, opts.PlacementOpts.Strategy)
			assert.Equal(t, StrategyBinpack, *opts.PlacementOpts.Strategy)
			assert.Equal(t, StrategyParamBinpackMemory, utility.FromStringPtr(opts.PlacementOpts.StrategyParameter))
		})
		t.Run("BadPlacementOptionsAreInvalid", func(t *testing.T) {
			placementOpts := NewECSPodPlacementOptions().SetStrategy("foo")
			opts := NewECSPodExecutionOptions().SetPlacementOptions(*placementOpts)
			assert.Error(t, opts.Validate())
		})
	})
}

func TestECSPodPlacementOptions(t *testing.T) {
	t.Run("NewECSPodPlacementOptions", func(t *testing.T) {
		opts := NewECSPodPlacementOptions()
		require.NotZero(t, opts)
		assert.Zero(t, *opts)
	})
	t.Run("SetStrategy", func(t *testing.T) {
		strategy := StrategyBinpack
		opts := NewECSPodPlacementOptions().SetStrategy(strategy)
		require.NotZero(t, opts.Strategy)
		assert.Equal(t, strategy, *opts.Strategy)
	})
	t.Run("SetStrategyParameter", func(t *testing.T) {
		param := StrategyParamBinpackCPU
		opts := NewECSPodPlacementOptions().SetStrategyParameter(param)
		assert.Equal(t, param, utility.FromStringPtr(opts.StrategyParameter))
	})
	t.Run("SetInstanceFilters", func(t *testing.T) {
		filters := []string{"runningTasksCount == 0"}
		opts := NewECSPodPlacementOptions().SetInstanceFilters(filters)
		assert.ElementsMatch(t, filters, opts.InstanceFilters)
	})
	t.Run("AddInstanceFilters", func(t *testing.T) {
		filter := "runningTasksCount == 0"
		opts := NewECSPodPlacementOptions().AddInstanceFilters(filter)
		require.Len(t, opts.InstanceFilters, 1)
		assert.Equal(t, filter, opts.InstanceFilters[0])

		opts.AddInstanceFilters()
		require.Len(t, opts.InstanceFilters, 1)
		assert.Equal(t, filter, opts.InstanceFilters[0])
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsValid", func(t *testing.T) {
			var opts ECSPodPlacementOptions
			assert.NoError(t, opts.Validate())
		})
		t.Run("EmptyDefaultsToBinpackMemory", func(t *testing.T) {
			var opts ECSPodPlacementOptions
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			require.NotZero(t, opts.StrategyParameter)
			assert.Equal(t, StrategyBinpack, *opts.Strategy)
			assert.Equal(t, StrategyParamBinpackMemory, *opts.StrategyParameter)
		})
		t.Run("BinpackWithoutParameterDefaultsToMemoryBinpacking", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategyBinpack)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, StrategyBinpack, *opts.Strategy)
			assert.Equal(t, StrategyParamBinpackMemory, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("BinpackWithMemoryBinpackingIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategyBinpack).SetStrategyParameter(StrategyParamBinpackMemory)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, StrategyBinpack, *opts.Strategy)
			assert.Equal(t, StrategyParamBinpackMemory, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("BinpackWithCPUBinpackingIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategyBinpack).SetStrategyParameter(StrategyParamBinpackCPU)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, StrategyBinpack, *opts.Strategy)
			assert.Equal(t, StrategyParamBinpackCPU, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("BinpackWithSpreadHostIsInvalid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategyBinpack).SetStrategyParameter(StrategyParamSpreadHost)
			assert.Error(t, opts.Validate())
		})
		t.Run("BinpackWithInvalidParameterIsInvalid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategyBinpack).SetStrategyParameter("foo")
			assert.Error(t, opts.Validate())
		})
		t.Run("SpreadyWithoutParameterDefaultsToHostSpread", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategySpread)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, StrategySpread, *opts.Strategy)
			assert.Equal(t, StrategyParamSpreadHost, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("SpreadWithHostSpreadIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategySpread).SetStrategyParameter(StrategyParamSpreadHost)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, StrategySpread, *opts.Strategy)
			assert.Equal(t, StrategyParamSpreadHost, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("SpreadWithCustomParameterIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(StrategySpread).SetStrategyParameter("custom")
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, StrategySpread, *opts.Strategy)
			assert.Equal(t, "custom", utility.FromStringPtr(opts.StrategyParameter))
		})
	})
}

func TestECSTaskDefinition(t *testing.T) {
	t.Run("NewECSTaskDefinition", func(t *testing.T) {
		def := NewECSTaskDefinition()
		require.NotZero(t, def)
		assert.Zero(t, *def)
	})
	t.Run("SetID", func(t *testing.T) {
		id := "id"
		def := NewECSTaskDefinition().SetID(id)
		assert.Equal(t, id, utility.FromStringPtr(def.ID))
	})
	t.Run("SetOwned", func(t *testing.T) {
		def := NewECSTaskDefinition().SetOwned(true)
		assert.True(t, utility.FromBoolPtr(def.Owned))
	})
}
