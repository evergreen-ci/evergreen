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
	t.Run("SetID", func(t *testing.T) {
		id := "id"
		s := NewContainerSecret().SetID(id)
		assert.Equal(t, id, utility.FromStringPtr(s.ID))
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		s := NewContainerSecret().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(s.Name))
	})
	t.Run("SetOwned", func(t *testing.T) {
		s := NewContainerSecret().SetOwned(true)
		assert.True(t, utility.FromBoolPtr(s.Owned))
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("SucceedsWithAllFieldsPopulated", func(t *testing.T) {
			s := NewContainerSecret().
				SetID("id").
				SetName("name").
				SetOwned(true)
			assert.NoError(t, s.Validate())
		})
		t.Run("SucceedsWithJustID", func(t *testing.T) {
			s := NewContainerSecret().SetID("id")
			assert.NoError(t, s.Validate())
		})
		t.Run("FailsWithEmpty", func(t *testing.T) {
			s := NewContainerSecret()
			assert.Error(t, s.Validate())
		})
		t.Run("FailsWithoutID", func(t *testing.T) {
			s := NewContainerSecret().SetName("name").SetOwned(true)
			assert.Error(t, s.Validate())
		})
		t.Run("FailsWithEmptyID", func(t *testing.T) {
			s := NewContainerSecret().SetID("")
			assert.Error(t, s.Validate())
		})
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
	t.Run("Validate", func(t *testing.T) {
		t.Run("SucceedsWithAllFieldsPopulated", func(t *testing.T) {
			opts := NewECSPodResources().
				SetTaskID("task_id").
				SetCluster("cluster").
				SetTaskDefinition(*NewECSTaskDefinition().SetID("task_definition_id")).
				AddContainers(*NewECSContainerResources().SetContainerID("container_id"))
			assert.NoError(t, opts.Validate())
		})
		t.Run("SucceedsWithJustTaskID", func(t *testing.T) {
			opts := NewECSPodResources().SetTaskID("task_id")
			assert.NoError(t, opts.Validate())
		})
		t.Run("FailsWithEmpty", func(t *testing.T) {
			assert.Error(t, NewECSPodResources().Validate())
		})
		t.Run("FailsWithInvalidTaskDefinition", func(t *testing.T) {
			opts := NewECSPodResources().
				SetTaskID("task_id").
				SetTaskDefinition(*NewECSTaskDefinition())
			assert.Error(t, opts.Validate())
		})
		t.Run("FailsWithInvalidContainer", func(t *testing.T) {
			opts := NewECSPodResources().
				SetTaskID("task_id").
				AddContainers(*NewECSContainerResources())
			assert.Error(t, opts.Validate())
		})
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
		secrets := []ContainerSecret{
			*NewContainerSecret().SetID("id0").SetName("name0"),
			*NewContainerSecret().SetID("id1").SetName("name1"),
		}
		res := NewECSContainerResources().SetSecrets(secrets)
		assert.ElementsMatch(t, secrets, res.Secrets)
		res.SetSecrets(nil)
		assert.Empty(t, res.Secrets)
	})
	t.Run("AddSecrets", func(t *testing.T) {
		secrets := []ContainerSecret{
			*NewContainerSecret().SetID("id0").SetName("name0"),
			*NewContainerSecret().SetID("id1").SetName("name1"),
		}
		res := NewECSContainerResources().AddSecrets(secrets...)
		assert.ElementsMatch(t, secrets, res.Secrets)
		res.AddSecrets()
		assert.ElementsMatch(t, secrets, res.Secrets)
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("SucceedsWithAllFieldsPopulated", func(t *testing.T) {
			r := NewECSContainerResources().
				SetContainerID("container_id").
				SetName("name").
				AddSecrets(*NewContainerSecret().SetID("id"))
			assert.NoError(t, r.Validate())
		})
		t.Run("FailsWithEmpty", func(t *testing.T) {
			r := NewECSContainerResources()
			assert.Error(t, r.Validate())
		})
		t.Run("SucceedsWithJustContainerID", func(t *testing.T) {
			r := NewECSContainerResources().SetContainerID("container_id")
			assert.NoError(t, r.Validate())
		})
		t.Run("FailsWithBadContainerID", func(t *testing.T) {
			r := NewECSContainerResources().SetContainerID("")
			assert.Error(t, r.Validate())
		})
		t.Run("FailsWithBadSecret", func(t *testing.T) {
			r := NewECSContainerResources().
				SetContainerID("container_id").
				AddSecrets(*NewContainerSecret())
			assert.Error(t, r.Validate())
		})
	})
}

func TestECSStatusInfo(t *testing.T) {
	t.Run("NewECSPodStatusInfo", func(t *testing.T) {
		ps := NewECSPodStatusInfo()
		require.NotZero(t, ps)
		assert.Zero(t, *ps)
	})
	t.Run("SetStatus", func(t *testing.T) {
		ps := NewECSPodStatusInfo().SetStatus(StatusRunning)
		assert.Equal(t, StatusRunning, ps.Status)
	})
	t.Run("SetContainers", func(t *testing.T) {
		cs := []ECSContainerStatusInfo{
			*NewECSContainerStatusInfo().
				SetContainerID("container_id0").
				SetName("container_name0").
				SetStatus(StatusRunning),
			*NewECSContainerStatusInfo().
				SetContainerID("container_id1").
				SetName("container_name1").
				SetStatus(StatusStopped),
		}
		ps := NewECSPodStatusInfo().SetContainers(cs)
		assert.ElementsMatch(t, ps.Containers, cs)
		ps.SetContainers(nil)
		assert.Empty(t, ps.Containers)
	})
	t.Run("AddContainers", func(t *testing.T) {
		cs := []ECSContainerStatusInfo{
			*NewECSContainerStatusInfo().
				SetContainerID("container_id0").
				SetName("container_name0").
				SetStatus(StatusRunning),
			*NewECSContainerStatusInfo().
				SetContainerID("container_id1").
				SetName("container_name1").
				SetStatus(StatusStopped),
		}

		ps := NewECSPodStatusInfo().AddContainers(cs...)
		assert.ElementsMatch(t, cs, ps.Containers)
		ps.AddContainers()
		assert.ElementsMatch(t, cs, ps.Containers)
	})
}

func TestECSContainerStatusInfo(t *testing.T) {
	t.Run("NewECSContainerStatusInfo", func(t *testing.T) {
		cs := NewECSContainerStatusInfo()
		require.NotZero(t, cs)
		assert.Zero(t, *cs)
	})
	t.Run("SetContainerID", func(t *testing.T) {
		id := "container_id"
		cs := NewECSContainerStatusInfo().SetContainerID(id)
		assert.Equal(t, id, utility.FromStringPtr(cs.ContainerID))
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		cs := NewECSContainerStatusInfo().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(cs.Name))
	})
	t.Run("SetStatus", func(t *testing.T) {
		status := StatusRunning
		cs := NewECSContainerStatusInfo().SetStatus(status)
		assert.Equal(t, status, cs.Status)
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
