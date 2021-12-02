package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestCreateConfig(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectConfigsCollection))
	projYml := `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
`
	pc, err := createConfig([]byte(projYml))
	assert.Nil(t, err)
	assert.Nil(t, pc)

	projYml = `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
deactivate_previous: true
`
	pc, err = createConfig([]byte(projYml))
	assert.Nil(t, err)
	assert.NotNil(t, pc)
	assert.True(t, utility.FromBoolPtr(pc.DeactivatePrevious))
}
