package scheduler

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheTaskGroups(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	require.NoError(db.ClearCollections(model.VersionCollection))
	tasks := []task.Task{}
	versions := []model.Version{}
	oddYml := `
task_groups:
- name: odd_task_group
  tasks:
`
	evenYml := `
task_groups:
- name: even_task_group
`
	versionMap := map[string]model.Version{}
	for i := 0; i < 10; i++ {
		versionId := fmt.Sprintf("version-%d", i)
		v := model.Version{
			Id: versionId,
		}

		if i%2 == 0 {
			v.Config = evenYml
		} else {
			v.Config = oddYml
		}
		require.NoError(v.Insert())
		versionMap[v.Id] = v
		versions = append(versions, v)
		tasks = append(tasks, task.Task{
			Id:      fmt.Sprintf("task-%d", i),
			Version: versionId,
		})
	}
	comparator := &CmpBasedTaskComparator{}
	comparator.tasks = tasks
	comparator.versions = versionMap
	assert.NoError(cacheTaskGroups(comparator))
	for i, v := range versions {
		if i%2 == 0 {
			assert.Equal("even_task_group", comparator.projects[v.Id].TaskGroups[0].Name)
		} else {
			assert.Equal("odd_task_group", comparator.projects[v.Id].TaskGroups[0].Name)
		}
	}
}

func TestBackfillTaskGroup(t *testing.T) {
	defer func() { require.NoError(t, db.ClearCollections(task.Collection)) }()
	tk := task.Task{
		Id:          "---",
		Version:     "ver",
		TaskGroup:   "grp",
		DisplayName: "one",
	}
	require.NoError(t, tk.Insert())
	tkm := task.Task{
		Id:          "foo",
		DisplayName: "zero",
	}
	require.NoError(t, tkm.Insert())
	cmp := &CmpBasedTaskComparator{
		tasks: []task.Task{tk, tkm},
		projects: map[string]project{
			tk.Version: project{
				TaskGroups: []model.TaskGroup{
					{
						MaxHosts: 42,
						Name:     "grp",
						Tasks:    []string{"", "for", "one"},
					},
				},
			},
		},
	}

	require.NoError(t, backfillTaskGroups(cmp))

	yes, err := task.FindOneId("---")
	require.NoError(t, err)
	no, err := task.FindOneId("foo")
	require.NoError(t, err)

	assert.Zero(t, no.TaskGroupMaxHosts)
	assert.Zero(t, no.TaskGroupOrder)
	assert.Equal(t, 42, yes.TaskGroupMaxHosts)
	assert.Equal(t, 3, yes.TaskGroupOrder)
}
