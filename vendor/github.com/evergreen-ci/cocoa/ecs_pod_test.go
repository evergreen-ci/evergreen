package cocoa

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerSecret(t *testing.T) {
	t.Run("NewContainerSecret", func(t *testing.T) {
		s := NewContainerSecret()
		require.NotZero(t, s)
		assert.Zero(t, *s)
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		s := NewContainerSecret().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(s.Name))
	})
	t.Run("SetValue", func(t *testing.T) {
		val := "val"
		s := NewContainerSecret().SetValue(val)
		assert.Equal(t, val, utility.FromStringPtr(s.Value))
	})
	t.Run("SetOwned", func(t *testing.T) {
		s := NewContainerSecret().SetOwned(true)
		assert.True(t, utility.FromBoolPtr(s.Owned))
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
	t.Run("SetCluster", func(t *testing.T) {
		cluster := "cluster"
		res := NewECSPodResources().SetCluster(cluster)
		require.NotZero(t, res.Cluster)
		assert.Equal(t, cluster, *res.Cluster)
	})
	t.Run("SetContainers", func(t *testing.T) {
		containerRes := NewECSContainerResources().SetContainerID("id").SetName("name")
		res := NewECSPodResources().SetContainers([]ECSContainerResources{*containerRes})
		require.Len(t, res.Containers, 1)
		assert.Equal(t, *containerRes, res.Containers[0])
	})
	t.Run("AddContainers", func(t *testing.T) {
		containerRes0 := NewECSContainerResources().SetContainerID("id0").SetName("name0")
		containerRes1 := NewECSContainerResources().SetContainerID("id1").SetName("name1")
		res := NewECSPodResources().AddContainers(*containerRes0, *containerRes1)
		require.Len(t, res.Containers, 2)
		assert.Equal(t, *containerRes0, res.Containers[0])
		assert.Equal(t, *containerRes1, res.Containers[1])
	})
}

func TestECSContainerResources(t *testing.T) {
	t.Run("NewECSContainerResources", func(t *testing.T) {
		res := NewECSContainerResources()
		require.NotZero(t, res)
		assert.Zero(t, *res)
	})
	t.Run("SetContainerID", func(t *testing.T) {
		id := "id"
		res := NewECSContainerResources().SetContainerID(id)
		require.NotZero(t, res.ContainerID)
		assert.Equal(t, id, utility.FromStringPtr(res.ContainerID))
	})
	t.Run("SetName", func(t *testing.T) {
		id := "id"
		res := NewECSContainerResources().SetName(id)
		require.NotZero(t, res.Name)
		assert.Equal(t, id, utility.FromStringPtr(res.Name))
	})
	t.Run("SetSecrets", func(t *testing.T) {
		s := NewContainerSecret().SetName("name").SetValue("value")
		res := NewECSContainerResources().SetSecrets([]ContainerSecret{*s})
		require.Len(t, res.Secrets, 1)
		assert.Equal(t, *s, res.Secrets[0])
	})
	t.Run("AddSecrets", func(t *testing.T) {
		s0 := NewContainerSecret().SetName("name0").SetValue("value0")
		s1 := NewContainerSecret().SetName("name1").SetValue("value1")
		res := NewECSContainerResources().AddSecrets(*s0, *s1)
		require.Len(t, res.Secrets, 2)
		assert.Equal(t, *s0, res.Secrets[0])
		assert.Equal(t, *s1, res.Secrets[1])
	})
}

func TestECSStatusInfo(t *testing.T) {
	t.Run("NewECSPodStatusInfo", func(t *testing.T) {
		stat := NewECSPodStatusInfo()
		require.NotZero(t, stat)
		assert.Zero(t, *stat)
	})
	t.Run("SetStatus", func(t *testing.T) {
		stat := NewECSPodStatusInfo().SetStatus(StatusRunning)
		assert.Equal(t, StatusRunning, stat.Status)
	})
	t.Run("SetContainers", func(t *testing.T) {
		containerStat0 := NewECSContainerStatusInfo().
			SetContainerID("container_id0").
			SetName("container_name0").
			SetStatus(StatusRunning)
		containerStat1 := NewECSContainerStatusInfo().
			SetContainerID("container_id1").
			SetName("container_name1").
			SetStatus(StatusRunning)
		stat := NewECSPodStatusInfo().SetContainers([]ECSContainerStatusInfo{
			*containerStat0, *containerStat1,
		})
		require.Len(t, stat.Containers, 2)
		assert.Equal(t, *containerStat0, stat.Containers[0])
		assert.Equal(t, *containerStat1, stat.Containers[1])
	})
	t.Run("AddContainers", func(t *testing.T) {
		containerStat := NewECSContainerStatusInfo().
			SetContainerID("container_id0").
			SetName("container_name0").
			SetStatus(StatusRunning)

		stat := NewECSPodStatusInfo().AddContainers(*containerStat)
		require.Len(t, stat.Containers, 1)
		assert.Equal(t, *containerStat, stat.Containers[0])

		stat.AddContainers()
		require.Len(t, stat.Containers, 1)
		assert.Equal(t, *containerStat, stat.Containers[0])
	})
}

func TestECSContainerStatusInfo(t *testing.T) {
	t.Run("NewECSContainerStatusInfo", func(t *testing.T) {
		stat := NewECSContainerStatusInfo()
		require.NotZero(t, stat)
		assert.Zero(t, *stat)
	})
	t.Run("SetContainerID", func(t *testing.T) {
		id := "container_id"
		stat := NewECSContainerStatusInfo().SetContainerID(id)
		assert.Equal(t, id, utility.FromStringPtr(stat.ContainerID))
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		stat := NewECSContainerStatusInfo().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(stat.Name))
	})
	t.Run("SetStatus", func(t *testing.T) {
		status := StatusRunning
		stat := NewECSContainerStatusInfo().SetStatus(status)
		assert.Equal(t, status, stat.Status)
	})
}

func TestECSStatus(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		for _, s := range []ECSStatus{
			StatusStarting,
			StatusRunning,
			StatusStopped,
			StatusDeleted,
			StatusUnknown,
		} {
			t.Run(fmt.Sprintf("SucceedsForStatus=%s", s), func(t *testing.T) {
				assert.NoError(t, s.Validate())
			})
		}
		t.Run("FailsForEmptyStatus", func(t *testing.T) {
			assert.Error(t, ECSStatus("").Validate())
		})
		t.Run("FailsForInvalidStatus", func(t *testing.T) {
			assert.Error(t, ECSStatus("invalid").Validate())
		})
	})
}
