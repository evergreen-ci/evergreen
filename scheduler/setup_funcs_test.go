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
