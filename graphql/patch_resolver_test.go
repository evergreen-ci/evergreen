package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestPatchTriggerAliases(t *testing.T) {
	ctx := context.Background()

	t.Run("ReturnsErrorWhenChildProjectHasNoVersions", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(
			model.ProjectRefCollection,
			model.VersionCollection,
			model.ParserProjectCollection,
		))

		// Create parent project with patch trigger alias pointing to child project
		parentProject := &model.ProjectRef{
			Id:         "parent-project",
			Identifier: "parent-project",
			PatchTriggerAliases: []patch.PatchTriggerDefinition{
				{
					Alias:        "test-alias",
					ChildProject: "child-project",
				},
			},
		}
		require.NoError(t, parentProject.Insert(t.Context()))

		// Create child project but do NOT create any versions for it
		childProject := &model.ProjectRef{
			Id:         "child-project",
			Identifier: "child-project",
		}
		require.NoError(t, childProject.Insert(t.Context()))

		// Create a patch object
		apiPatch := &restModel.APIPatch{
			ProjectId: utility.ToStringPtr("parent-project"),
			Requester: utility.ToStringPtr(evergreen.PatchVersionRequester),
		}

		// Call the resolver
		resolver := &patchResolver{}
		result, err := resolver.PatchTriggerAliases(ctx, apiPatch)

		// Should return an error because child project has no versions
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "did not find valid version for project 'child-project'")
	})

	t.Run("SucceedsWhenChildProjectHasVersions", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(
			model.ProjectRefCollection,
			model.VersionCollection,
			model.ParserProjectCollection,
		))

		// Create parent project with patch trigger alias
		parentProject := &model.ProjectRef{
			Id:         "parent-project",
			Identifier: "parent-project",
			PatchTriggerAliases: []patch.PatchTriggerDefinition{
				{
					Alias:        "test-alias",
					ChildProject: "child-project",
					TaskSpecifiers: []patch.TaskSpecifier{
						{
							PatchAlias: "child-alias",
						},
					},
				},
			},
		}
		require.NoError(t, parentProject.Insert(t.Context()))

		// Create child project
		childProject := &model.ProjectRef{
			Id:         "child-project",
			Identifier: "child-project",
		}
		require.NoError(t, childProject.Insert(t.Context()))

		// Create a version for the child project
		childVersion := &model.Version{
			Id:         "child-version-1",
			Identifier: "child-project",
			Requester:  evergreen.RepotrackerVersionRequester,
		}
		require.NoError(t, childVersion.Insert(t.Context()))

		// Create a parser project for the child version with a simple config
		pp := &model.ParserProject{
			Id: "child-version-1",
		}
		yamlConfig := `
buildvariants:
- name: test-variant
  display_name: Test Variant
  run_on:
  - ubuntu1604-small
  tasks:
  - name: test-task

tasks:
- name: test-task
  commands:
  - command: shell.exec
    params:
      script: echo "test"
`
		require.NoError(t, utility.UnmarshalYAMLWithFallback([]byte(yamlConfig), &pp))
		pp.Id = "child-version-1"
		require.NoError(t, pp.Insert(t.Context()))

		// Create a patch object
		apiPatch := &restModel.APIPatch{
			ProjectId: utility.ToStringPtr("parent-project"),
			Requester: utility.ToStringPtr(evergreen.PatchVersionRequester),
		}

		// Call the resolver - should succeed now
		resolver := &patchResolver{}
		result, err := resolver.PatchTriggerAliases(gimlet.AttachUser(ctx, &testutil.MockUser{}), apiPatch)

		// Should succeed
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, "test-alias", utility.FromStringPtr(result[0].Alias))
		assert.Equal(t, "child-project", utility.FromStringPtr(result[0].ChildProjectIdentifier))
	})
}
