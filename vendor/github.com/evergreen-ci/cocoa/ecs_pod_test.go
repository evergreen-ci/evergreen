package cocoa

import (
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodSecret(t *testing.T) {
	t.Run("NewPodSecret", func(t *testing.T) {
		s := NewPodSecret()
		require.NotZero(t, s)
		assert.Zero(t, *s)
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		s := NewPodSecret().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(s.Name))
	})
	t.Run("SetValue", func(t *testing.T) {
		val := "val"
		s := NewPodSecret().SetValue(val)
		assert.Equal(t, val, utility.FromStringPtr(s.Value))
	})
}

func TestECSPodResources(t *testing.T) {
	t.Run("NewECSPodResources", func(t *testing.T) {
		res := NewECSPodResources()
		require.NotZero(t, res)
		assert.Zero(t, *res)
	})
	t.Run("SetTaskID", func(t *testing.T) {
		id := "id"
		res := NewECSPodResources().SetTaskID(id)
		assert.Equal(t, id, utility.FromStringPtr(res.TaskID))
	})
	t.Run("SetTaskDefinition", func(t *testing.T) {
		def := NewECSTaskDefinition().SetID("id")
		res := NewECSPodResources().SetTaskDefinition(*def)
		require.NotZero(t, res.TaskDefinition)
		assert.Equal(t, *def, *res.TaskDefinition)
	})
	t.Run("SetSecrets", func(t *testing.T) {
		s := NewPodSecret().SetName("name").SetValue("value")
		res := NewECSPodResources().SetSecrets([]PodSecret{*s})
		require.Len(t, res.Secrets, 1)
		assert.Equal(t, *s, res.Secrets[0])
	})
	t.Run("AddSecrets", func(t *testing.T) {
		s0 := NewPodSecret().SetName("name0").SetValue("value0")
		s1 := NewPodSecret().SetName("name1").SetValue("value1")
		res := NewECSPodResources().AddSecrets(*s0, *s1)
		require.Len(t, res.Secrets, 2)
		assert.Equal(t, *s0, res.Secrets[0])
		assert.Equal(t, *s1, res.Secrets[1])
	})
	t.Run("SetCluster", func(t *testing.T) {
		cluster := "cluster"
		res := NewECSPodResources().SetCluster(cluster)
		require.NotZero(t, res.Cluster)
		assert.Equal(t, cluster, *res.Cluster)
	})
}
