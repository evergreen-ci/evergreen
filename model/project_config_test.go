package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateConfig(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectConfigCollection))
	projYml := `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
create_time: 2022-12-15T17:18:32Z
`
	pc, err := CreateProjectConfig([]byte(projYml), "")
	assert.NoError(t, err)
	assert.Nil(t, pc)

	projYml = `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
build_baron_settings:
  ticket_create_project: BF
  ticket_create_issue_type: Bug
  ticket_search_projects: ["BF"]

github_pr_aliases:
  - variant: "^ubuntu1604$"
    task: ".*"

`
	pc, err = CreateProjectConfig([]byte(projYml), "")
	assert.NoError(t, err)
	assert.NotNil(t, pc)
	assert.Equal(t, []string{"BF"}, pc.BuildBaronSettings.TicketSearchProjects)
	assert.Equal(t, "BF", pc.BuildBaronSettings.TicketCreateProject)
	assert.Equal(t, "Bug", pc.BuildBaronSettings.TicketCreateIssueType)

	projYml = `
patch_trigger_aliases:
  - alias: downstream-trigger
    child_project: my-downstream-project
    task_specifiers:
      - patch_alias: my-patch-alias
      - task_regex: "^lint.*"
        variant_regex: "^ubuntu"
    status: success
    parent_as_module: my-module
    downstream_revision: abc123
`
	pc, err = CreateProjectConfig([]byte(projYml), "my-project")
	require.NoError(t, err)
	require.NotNil(t, pc)
	assert.Equal(t, "my-project", pc.Project)
	require.Len(t, pc.PatchTriggerAliases, 1)
	pta := pc.PatchTriggerAliases[0]
	assert.Equal(t, "downstream-trigger", pta.Alias)
	assert.Equal(t, "my-downstream-project", pta.ChildProject)
	assert.Equal(t, "success", pta.Status)
	assert.Equal(t, "my-module", pta.ParentAsModule)
	assert.Equal(t, "abc123", pta.DownstreamRevision)
	require.Len(t, pta.TaskSpecifiers, 2)
	assert.Equal(t, "my-patch-alias", pta.TaskSpecifiers[0].PatchAlias)
	assert.Empty(t, pta.TaskSpecifiers[0].TaskRegex)
	assert.Equal(t, "^lint.*", pta.TaskSpecifiers[1].TaskRegex)
	assert.Equal(t, "^ubuntu", pta.TaskSpecifiers[1].VariantRegex)

	projYml = `
patch_trigger_aliases:
  - alias: trigger1
    child_project: project-a
    task_specifiers:
      - patch_alias: alias-a
  - alias: trigger2
    child_project: project-b
    status: "*"
    task_specifiers:
      - task_regex: ".*"
        variant_regex: ".*"
`
	pc, err = CreateProjectConfig([]byte(projYml), "")
	require.NoError(t, err)
	require.NotNil(t, pc)
	require.Len(t, pc.PatchTriggerAliases, 2)
	assert.Equal(t, "trigger1", pc.PatchTriggerAliases[0].Alias)
	assert.Equal(t, "project-a", pc.PatchTriggerAliases[0].ChildProject)
	assert.Equal(t, "trigger2", pc.PatchTriggerAliases[1].Alias)
	assert.Equal(t, "project-b", pc.PatchTriggerAliases[1].ChildProject)
	assert.Equal(t, "*", pc.PatchTriggerAliases[1].Status)
}
