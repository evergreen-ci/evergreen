package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaterfallTaskTags(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection))
	})

	versions := make([]Version, 3)
	for i := range versions {
		id := fmt.Sprintf("v%d", i+1)
		versions[i] = Version{
			Id: id, Identifier: "project", RevisionOrderNumber: i + 1,
			Requester: evergreen.RepotrackerVersionRequester, Activated: utility.TruePtr(),
			BuildIds: []string{id + "_build"},
		}
		require.NoError(t, versions[i].Insert(t.Context()))
		b := build.Build{Id: id + "_build", Version: id, BuildVariant: "linux", DisplayName: "Linux", Activated: true}
		require.NoError(t, b.Insert(t.Context()))
	}
	tasks := []task.Task{
		{Id: "display", Version: "v1", RevisionOrderNumber: 1, DisplayName: "Display", DisplayOnly: true,
			ExecutionTasks: []string{"child", "other_child"}, DisplayStatusCache: evergreen.TaskFailed, Activated: true},
		{Id: "child", Version: "v1", RevisionOrderNumber: 1, DisplayName: "Child", DisplayTaskId: utility.ToStringPtr("display"),
			Tags: []string{"integration", "shared"}, DisplayStatusCache: evergreen.TaskSucceeded},
		{Id: "other_child", Version: "v1", RevisionOrderNumber: 1, DisplayName: "Other child", DisplayTaskId: utility.ToStringPtr("display"),
			Tags: []string{"shared", "performance"}, DisplayStatusCache: evergreen.TaskFailed, Activated: true},
		{Id: "untagged", Version: "v1", RevisionOrderNumber: 1, DisplayName: "Untagged", Activated: true},
		{Id: "regular", Version: "v2", RevisionOrderNumber: 2, DisplayName: "Regular", Tags: []string{"integration"},
			DisplayStatusCache: evergreen.TaskSucceeded, Activated: true},
		{Id: "unrelated", Version: "v3", RevisionOrderNumber: 3, DisplayName: "Unrelated", Tags: []string{"unit"}, Activated: true},
	}
	for _, tsk := range tasks {
		tsk.Project = "project"
		tsk.BuildId = tsk.Version + "_build"
		tsk.BuildVariant = "linux"
		tsk.Requester = evergreen.RepotrackerVersionRequester
		require.NoError(t, tsk.Insert(t.Context()))
	}

	t.Run("BuildsIncludeUnionOfChildTagsAndExcludeExecutionTasks", func(t *testing.T) {
		builds, err := GetVersionBuilds(t.Context(), versions[0], WaterfallOptions{IncludeTaskTags: true})
		require.NoError(t, err)
		require.Len(t, builds, 1)
		require.Len(t, builds[0].Tasks, 2)
		assert.Equal(t, "display", builds[0].Tasks[0].Id)
		assert.ElementsMatch(t, []string{"integration", "shared", "performance"}, builds[0].Tasks[0].Tags)
		assert.Empty(t, builds[0].Tasks[1].Tags)
	})

	for name, test := range map[string]struct {
		opts WaterfallOptions
		want []string
	}{
		"ChildTagMatchesDisplayTask":                 {WaterfallOptions{TaskTags: []string{"integration"}}, []string{"display"}},
		"MultipleTagsMatchAnyAndDoNotDuplicateTasks": {WaterfallOptions{TaskTags: []string{"integration", "performance"}}, []string{"display"}},
		"TagsMatchExactly":                           {WaterfallOptions{TaskTags: []string{"integr.*"}}, nil},
		"TagsAreCaseSensitive":                       {WaterfallOptions{TaskTags: []string{"Integration"}}, nil},
		"UnknownTagsReturnNoBuilds":                  {WaterfallOptions{TaskTags: []string{"unknown"}}, nil},
		"TagAndNameAndStatusMatchParent":             {WaterfallOptions{TaskTags: []string{"integration"}, Tasks: []string{"^Display$"}, Statuses: []string{evergreen.TaskFailed}}, []string{"display"}},
		"ChildStatusDoesNotOverrideParentStatus":     {WaterfallOptions{TaskTags: []string{"integration"}, Statuses: []string{evergreen.TaskSucceeded}}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			builds, err := GetVersionBuilds(t.Context(), versions[0], test.opts)
			require.NoError(t, err)
			var ids []string
			for _, b := range builds {
				for _, tsk := range b.Tasks {
					ids = append(ids, tsk.Id)
				}
			}
			assert.Equal(t, test.want, ids)
		})
	}

	for name, test := range map[string]struct {
		opts   WaterfallOptions
		offset int
		want   []string
	}{
		"VersionsMatchRegularAndDisplayTasks":    {WaterfallOptions{TaskTags: []string{"integration"}}, 4, []string{"v2", "v1"}},
		"VersionTagFiltersMatchAny":              {WaterfallOptions{TaskTags: []string{"integration", "unit"}}, 4, []string{"v3", "v2", "v1"}},
		"VersionTagFiltersAreExact":              {WaterfallOptions{TaskTags: []string{"integr.*"}}, 4, nil},
		"VersionTagFiltersAreCaseSensitive":      {WaterfallOptions{TaskTags: []string{"Integration"}}, 4, nil},
		"VersionFiltersMatchParentNameAndStatus": {WaterfallOptions{TaskTags: []string{"integration"}, Tasks: []string{"^Display$"}, Statuses: []string{evergreen.TaskFailed}}, 4, []string{"v1"}},
		"VersionFiltersDoNotMatchChildStatus":    {WaterfallOptions{TaskTags: []string{"integration"}, Tasks: []string{"^Display$"}, Statuses: []string{evergreen.TaskSucceeded}}, 4, nil},
		"TagAndVariantFiltersIntersect":          {WaterfallOptions{TaskTags: []string{"integration"}, Variants: []string{"windows"}}, 4, nil},
		"ForwardPaginationFindsOlderTagMatches":  {WaterfallOptions{TaskTags: []string{"integration"}, MaxOrder: 2}, 2, []string{"v1"}},
		"BackwardPaginationFindsNewerTagMatches": {WaterfallOptions{TaskTags: []string{"integration"}, MinOrder: 1}, 1, []string{"v2"}},
	} {
		t.Run(name, func(t *testing.T) {
			opts := test.opts
			opts.Limit = 5
			opts.Requesters = evergreen.SystemVersionRequesterTypes
			found, err := GetActiveVersionsByTaskFilters(t.Context(), "project", opts, test.offset)
			require.NoError(t, err)
			var ids []string
			for _, v := range found {
				ids = append(ids, v.Id)
			}
			assert.Equal(t, test.want, ids)
		})
	}
}
