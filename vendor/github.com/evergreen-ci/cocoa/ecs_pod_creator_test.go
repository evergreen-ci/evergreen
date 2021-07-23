package cocoa

import (
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
		def := NewECSPodCreationOptions().SetContainerDefinitions([]ECSContainerDefinition{*containerDef})
		require.Len(t, def.ContainerDefinitions, 1)
		assert.Equal(t, *containerDef, def.ContainerDefinitions[0])
	})
	t.Run("AddContainerDefinitions", func(t *testing.T) {
		containerDef0 := NewECSContainerDefinition().SetImage("image0")
		containerDef1 := NewECSContainerDefinition().SetImage("image1")
		def := NewECSPodCreationOptions().AddContainerDefinitions(*containerDef0, *containerDef1)
		require.Len(t, def.ContainerDefinitions, 2)
		assert.Equal(t, *containerDef0, def.ContainerDefinitions[0])
		assert.Equal(t, *containerDef1, def.ContainerDefinitions[1])
		def.AddContainerDefinitions()
		assert.Len(t, def.ContainerDefinitions, 2)
	})
	t.Run("SetMemoryMB", func(t *testing.T) {
		mem := 128
		def := NewECSPodCreationOptions().SetMemoryMB(mem)
		assert.Equal(t, mem, utility.FromIntPtr(def.MemoryMB))
	})
	t.Run("SetCPU", func(t *testing.T) {
		cpu := 128
		def := NewECSPodCreationOptions().SetCPU(cpu)
		assert.Equal(t, cpu, utility.FromIntPtr(def.CPU))
	})
	t.Run("SetTags", func(t *testing.T) {
		key := "tagKey"
		value := "tagValue"
		def := NewECSPodCreationOptions().SetTags(map[string]string{key: value})
		require.Len(t, def.Tags, 1)
		assert.Equal(t, value, def.Tags[key])
	})
	t.Run("AddTags", func(t *testing.T) {
		key0 := "key0"
		val0 := "val0"
		key1 := "key1"
		val1 := "val1"
		def := NewECSPodCreationOptions().AddTags(map[string]string{key0: val0, key1: val1})
		require.Len(t, def.Tags, 2)
		assert.Equal(t, val0, def.Tags[key0])
		assert.Equal(t, val1, def.Tags[key1])
		def.AddTags(map[string]string{})
		assert.Len(t, def.Tags, 2)
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			assert.Error(t, NewECSPodCreationOptions().Validate())
		})
		t.Run("MemoryCPUAndContainerDefinitionIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128)
			assert.NoError(t, def.Validate())
		})
		t.Run("MissingContainerDefinitionsIsInvalid", func(t *testing.T) {
			def := NewECSPodCreationOptions().
				SetMemoryMB(128).
				SetCPU(128)
			assert.Error(t, def.Validate())
		})
		t.Run("NameIsGenerated", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128)
			assert.NoError(t, def.Validate())
			assert.NotZero(t, utility.FromStringPtr(def.Name))
		})
		t.Run("BadContainerDefinitionIsInvalid", func(t *testing.T) {
			def := NewECSPodCreationOptions().AddContainerDefinitions(*NewECSContainerDefinition()).SetMemoryMB(128).SetCPU(128)
			assert.Error(t, def.Validate())
		})
		t.Run("AllFieldsPopulatedIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			def := NewECSPodCreationOptions().
				SetName("name").
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetTaskRole("role").
				AddTags(map[string]string{"key": "val"}).
				SetExecutionOptions(*NewECSPodExecutionOptions())
			assert.NoError(t, def.Validate())
		})
		t.Run("MissingCPUIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128)
			assert.Error(t, def.Validate())
		})
		t.Run("MissingPodCPUWithContainerCPUIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetCPU(128)
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128)
			assert.NoError(t, def.Validate())
		})
		t.Run("TotalContainerCPUCannotExceedPodCPU", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetCPU(256)
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(1024).
				SetCPU(128)
			assert.Error(t, def.Validate())
		})
		t.Run("ZeroCPUIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(0)
			assert.Error(t, def.Validate())
		})
		t.Run("MissingMemoryIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetCPU(128)
			assert.Error(t, def.Validate())
		})
		t.Run("MissingPodMemoryWithContainerMemoryIsValid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetMemoryMB(128)
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetCPU(128)
			assert.NoError(t, def.Validate())
		})
		t.Run("TotalContainerMemoryCannotExceedPodMemory", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image").SetMemoryMB(256)
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(1024)
			assert.Error(t, def.Validate())
		})
		t.Run("ZeroMemoryIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(0).
				SetCPU(128)
			assert.Error(t, def.Validate())
		})
		t.Run("BadExecutionOptionsIsInvalid", func(t *testing.T) {
			containerDef := NewECSContainerDefinition().SetImage("image")
			placementOpts := NewECSPodPlacementOptions().SetStrategy("foo")
			execOpts := NewECSPodExecutionOptions().SetPlacementOptions(*placementOpts)
			def := NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetExecutionOptions(*execOpts)
			assert.Error(t, def.Validate())
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
	t.Run("SetTags", func(t *testing.T) {
		tag := "tag"
		def := NewECSContainerDefinition().SetTags([]string{tag})
		require.Len(t, def.Tags, 1)
		assert.Equal(t, tag, def.Tags[0])
	})
	t.Run("AddTags", func(t *testing.T) {
		tag0 := "tag0"
		tag1 := "tag1"
		def := NewECSContainerDefinition().AddTags(tag0, tag1)
		require.Len(t, def.Tags, 2)
		assert.Equal(t, tag0, def.Tags[0])
		assert.Equal(t, tag1, def.Tags[1])
		def.AddTags()
		assert.Len(t, def.Tags, 2)
	})
	t.Run("SetEnvironmentVariables", func(t *testing.T) {
		ev := NewEnvironmentVariable().SetName("name").SetValue("value")
		def := NewECSContainerDefinition().SetEnvironmentVariables([]EnvironmentVariable{*ev})
		require.Len(t, def.EnvVars, 1)
		assert.Equal(t, *ev, def.EnvVars[0])
	})
	t.Run("AddEnvironmentVariables", func(t *testing.T) {
		ev0 := NewEnvironmentVariable().SetName("name0").SetValue("value0")
		ev1 := NewEnvironmentVariable().SetName("name1").SetValue("value1")
		def := NewECSContainerDefinition().AddEnvironmentVariables(*ev0, *ev1)
		require.Len(t, def.EnvVars, 2)
		assert.Equal(t, *ev0, def.EnvVars[0])
		assert.Equal(t, *ev1, def.EnvVars[1])
		def.AddEnvironmentVariables()
		assert.Len(t, def.EnvVars, 2)
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
				AddEnvironmentVariables(*ev).
				AddTags("tag")
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
		assert.Equal(t, opts.Name, ev.SecretOpts.Name)
		assert.Equal(t, opts.Value, ev.SecretOpts.Value)
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
		placementOpts := NewECSPodPlacementOptions().SetStrategy(BinpackPlacement)
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
	})
	t.Run("SetExecutionRole", func(t *testing.T) {
		role := "role"
		opts := NewECSPodExecutionOptions().SetExecutionRole(role)
		assert.Equal(t, role, *opts.ExecutionRole)
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
			assert.Equal(t, BinpackPlacement, *opts.PlacementOpts.Strategy)
			assert.Equal(t, BinpackMemory, utility.FromStringPtr(opts.PlacementOpts.StrategyParameter))
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
		strategy := BinpackPlacement
		opts := NewECSPodPlacementOptions().SetStrategy(strategy)
		require.NotZero(t, opts.Strategy)
		assert.Equal(t, strategy, *opts.Strategy)
	})
	t.Run("SetStrategyParameter", func(t *testing.T) {
		param := BinpackCPU
		opts := NewECSPodPlacementOptions().SetStrategyParameter(param)
		assert.Equal(t, param, utility.FromStringPtr(opts.StrategyParameter))
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
			assert.Equal(t, BinpackPlacement, *opts.Strategy)
			assert.Equal(t, BinpackMemory, *opts.StrategyParameter)
		})
		t.Run("BinpackStrategyWithoutParameterDefaultsToMemoryBinpacking", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(BinpackPlacement)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, BinpackPlacement, *opts.Strategy)
			assert.Equal(t, BinpackMemory, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("BinpackStrategyWithMemoryBinpackingIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(BinpackPlacement).SetStrategyParameter(BinpackMemory)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, BinpackPlacement, *opts.Strategy)
			assert.Equal(t, BinpackMemory, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("BinpackStrategyWithCPUBinpackingIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(BinpackPlacement).SetStrategyParameter(BinpackCPU)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, BinpackPlacement, *opts.Strategy)
			assert.Equal(t, BinpackCPU, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("BinpackStrategyWithSpreadHostIsInvalid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(BinpackPlacement).SetStrategyParameter(SpreadHost)
			assert.Error(t, opts.Validate())
		})
		t.Run("BinpackStrategyWithInvalidParameterIsInvalid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(BinpackPlacement).SetStrategyParameter("foo")
			assert.Error(t, opts.Validate())
		})
		t.Run("SpreadStrategyWithoutParameterDefaultsToHostSpread", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(SpreadPlacement)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, SpreadPlacement, *opts.Strategy)
			assert.Equal(t, SpreadHost, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("SpreadStrategyWithoutWithHostSpreadIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(SpreadPlacement).SetStrategyParameter(SpreadHost)
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, SpreadPlacement, *opts.Strategy)
			assert.Equal(t, SpreadHost, utility.FromStringPtr(opts.StrategyParameter))
		})
		t.Run("SpreadStrategyWithCustomParameterIsValid", func(t *testing.T) {
			opts := NewECSPodPlacementOptions().SetStrategy(SpreadPlacement).SetStrategyParameter("custom")
			require.NoError(t, opts.Validate())
			require.NotZero(t, opts.Strategy)
			assert.Equal(t, SpreadPlacement, *opts.Strategy)
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
