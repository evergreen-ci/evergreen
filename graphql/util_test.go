package graphql

import (
	"context"
	"fmt"
	"testing"
	"time"

	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/graphql/loaders"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func init() {
	testutil.Setup()
}

func TestFilterGeneralSubscriptions(t *testing.T) {
	usr := &user.DBUser{}
	usr.Settings.Notifications = user.NotificationPreferences{
		PatchFinishID:         "patch_finish_id",
		SpawnHostExpirationID: "spawn_host_subscription_id",
	}

	t.Run("NoGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "123455"},
			{ID: "abcdef"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.ElementsMatch(t, []string{"123455", "abcdef"}, filteredSubIDs)
	})

	t.Run("OnlyGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "patch_finish_id"},
			{ID: "spawn_host_subscription_id"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.Empty(t, filteredSubIDs)
	})

	t.Run("MixGeneralSubscriptions", func(t *testing.T) {
		subs := []event.Subscription{
			{ID: "patch_finish_id"},
			{ID: "123456"},
		}
		filteredSubIDs := removeGeneralSubscriptions(usr, subs)
		assert.ElementsMatch(t, []string{"123456"}, filteredSubIDs)
	})
}

func TestCanRestartTask(t *testing.T) {
	ctx := t.Context()

	blockedTask := &task.Task{
		Id: "t1",
		DependsOn: []task.Dependency{
			{TaskId: "testDepends1", Status: "*", Unattainable: true},
			{TaskId: "testDepends2", Status: "*", Unattainable: false},
		},
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canRestart := canRestartTask(ctx, blockedTask)
	assert.False(t, canRestart)

	blockedDisplayTask := &task.Task{
		Id:             "t4",
		Status:         evergreen.TaskUndispatched,
		DisplayStatus:  evergreen.TaskStatusBlocked,
		DisplayTaskId:  utility.ToStringPtr(""),
		DisplayOnly:    true,
		ExecutionTasks: []string{"exec1", "exec2"},
	}
	canRestart = canRestartTask(ctx, blockedDisplayTask)
	assert.True(t, canRestart)

	executionTask := &task.Task{
		Id:            "t2",
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr("display task"),
	}
	canRestart = canRestartTask(ctx, executionTask)
	assert.False(t, canRestart)

	runningTask := &task.Task{
		Id:            "t3",
		Status:        evergreen.TaskStarted,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canRestart = canRestartTask(ctx, runningTask)
	assert.False(t, canRestart)

	finishedTask := &task.Task{
		Id:            "t5",
		Status:        evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canRestart = canRestartTask(ctx, finishedTask)
	assert.True(t, canRestart)

	abortedTask := &task.Task{
		Id:            "t6",
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr(""),
		Aborted:       true,
	}
	canRestart = canRestartTask(ctx, abortedTask)
	assert.False(t, canRestart)
}

func TestCanScheduleTask(t *testing.T) {
	ctx := t.Context()

	abortedTask := &task.Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		DisplayTaskId: utility.ToStringPtr(""),
		Aborted:       true,
	}
	canSchedule := canScheduleTask(ctx, abortedTask)
	assert.False(t, canSchedule)

	executionTask := &task.Task{
		Id:            "t2",
		Status:        evergreen.TaskUndispatched,
		DisplayStatus: evergreen.TaskUnscheduled,
		DisplayTaskId: utility.ToStringPtr("display task"),
	}
	canSchedule = canScheduleTask(ctx, executionTask)
	assert.False(t, canSchedule)

	finishedTask := &task.Task{
		Id:            "t4",
		Status:        evergreen.TaskSucceeded,
		DisplayStatus: evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canSchedule = canScheduleTask(ctx, finishedTask)
	assert.False(t, canSchedule)

	unscheduledTask := &task.Task{
		Id:            "t3",
		Status:        evergreen.TaskUndispatched,
		DisplayStatus: evergreen.TaskUnscheduled,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	canSchedule = canScheduleTask(ctx, unscheduledTask)
	assert.True(t, canSchedule)
}

func TestGetDisplayStatus(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.VersionCollection, patch.Collection))
	patchId := mgobson.NewObjectId()
	childPatchId := mgobson.NewObjectId()
	version := &model.Version{
		Id:        patchId.Hex(),
		Aborted:   true,
		Status:    evergreen.VersionSucceeded,
		Requester: evergreen.PatchVersionRequester,
	}

	assert.NoError(t, version.Insert(t.Context()))

	p := &patch.Patch{
		Id:     patchId,
		Status: evergreen.VersionSucceeded,
		Triggers: patch.TriggerInfo{
			ChildPatches: []string{childPatchId.Hex()},
		},
	}
	assert.NoError(t, p.Insert(t.Context()))

	cv := model.Version{
		Id:      childPatchId.Hex(),
		Aborted: true,
		Status:  evergreen.VersionFailed,
	}
	assert.NoError(t, cv.Insert(t.Context()))

	cp := &patch.Patch{
		Id:     childPatchId,
		Status: evergreen.VersionFailed,
	}
	assert.NoError(t, cp.Insert(t.Context()))

	status, err := getDisplayStatus(loaders.Inject(t.Context()), version)
	require.NoError(t, err)
	assert.Equal(t, evergreen.VersionAborted, status)
}

func TestConcurrentlyBuildVersionsMatchingTasksMap(t *testing.T) {
	ctx := context.Background()
	assert.NoError(t, db.ClearCollections(task.Collection))

	t1 := task.Task{
		Id:                      "t1",
		DisplayName:             "test-agent",
		Version:                 "v1",
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		Execution:               0,
		Status:                  evergreen.TaskSucceeded,
		DisplayStatusCache:      evergreen.TaskSucceeded,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t2 := task.Task{
		Id:                      "t2",
		DisplayName:             "test-model",
		Version:                 "v1",
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		Execution:               0,
		Status:                  evergreen.TaskFailed,
		DisplayStatusCache:      evergreen.TaskFailed,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t3 := task.Task{
		Id:                      "t3",
		DisplayName:             "test-rest-route",
		Version:                 "v1",
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		Execution:               1,
		Status:                  evergreen.TaskSucceeded,
		DisplayStatusCache:      evergreen.TaskSucceeded,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t4 := task.Task{
		Id:                      "t4",
		DisplayName:             "test-rest-data",
		Version:                 "v1",
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		Execution:               1,
		Status:                  evergreen.TaskFailed,
		DisplayStatusCache:      evergreen.TaskFailed,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t5 := task.Task{
		Id:                      "t5",
		DisplayName:             "test-model",
		Version:                 "v2",
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		Execution:               1,
		Status:                  evergreen.TaskFailed,
		DisplayStatusCache:      evergreen.TaskFailed,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t6 := task.Task{
		Id:                      "t6",
		DisplayName:             "test-model",
		Version:                 "v3",
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		Execution:               1,
		Status:                  evergreen.TaskFailed,
		DisplayStatusCache:      evergreen.TaskFailed,
		DisplayTaskId:           utility.ToStringPtr(""),
	}

	assert.NoError(t, db.InsertMany(t.Context(), task.Collection, t1, t2, t3, t4, t5, t6))

	opts := task.HasMatchingTasksOptions{
		TaskNames:                  []string{"agent"},
		Variants:                   []string{},
		Statuses:                   []string{},
		IncludeNeverActivatedTasks: true,
	}

	versions := []model.Version{
		{
			Id:        "v1",
			Activated: utility.TruePtr(),
		},
		{
			Id:        "v2",
			Activated: utility.TruePtr(),
		},
		{
			Id:        "v3",
			Activated: utility.TruePtr(),
		},
	}

	versionsMatchingTasksMap, err := concurrentlyBuildVersionsMatchingTasksMap(ctx, versions, opts)
	assert.NoError(t, err)
	assert.NotNil(t, versionsMatchingTasksMap)
	assert.True(t, versionsMatchingTasksMap["v1"])
	assert.False(t, versionsMatchingTasksMap["v2"])
	assert.False(t, versionsMatchingTasksMap["v3"])

	opts = task.HasMatchingTasksOptions{
		TaskNames:                  []string{},
		Variants:                   []string{"bv1"},
		Statuses:                   []string{},
		IncludeNeverActivatedTasks: true,
	}

	versionsMatchingTasksMap, err = concurrentlyBuildVersionsMatchingTasksMap(ctx, versions, opts)
	assert.NoError(t, err)
	assert.NotNil(t, versionsMatchingTasksMap)
	assert.True(t, versionsMatchingTasksMap["v1"])
	assert.True(t, versionsMatchingTasksMap["v2"])
	assert.False(t, versionsMatchingTasksMap["v3"])

	opts = task.HasMatchingTasksOptions{
		TaskNames:                  []string{"model"},
		Variants:                   []string{"v2"},
		Statuses:                   []string{evergreen.TaskFailed},
		IncludeNeverActivatedTasks: true,
	}

	versionsMatchingTasksMap, err = concurrentlyBuildVersionsMatchingTasksMap(ctx, versions, opts)
	assert.NoError(t, err)
	assert.NotNil(t, versionsMatchingTasksMap)
	assert.False(t, versionsMatchingTasksMap["v1"])
	assert.False(t, versionsMatchingTasksMap["v2"])
	assert.True(t, versionsMatchingTasksMap["v3"])

}
func TestIsPatchAuthorForTask(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"TrueWhenUserIsPatchAuthor": func(ctx context.Context, t *testing.T) {
			versionAndPatchID := mgobson.NewObjectId()
			patch := patch.Patch{
				Id:     versionAndPatchID,
				Author: "basic_user",
			}
			assert.NoError(t, patch.Insert(t.Context()))
			task := restModel.APITask{ProjectId: utility.ToStringPtr("random_project_id"), Version: utility.ToStringPtr(versionAndPatchID.Hex()), Requester: utility.ToStringPtr(evergreen.PatchVersionRequester)}
			isPatchAuthor, err := isPatchAuthorForTask(ctx, &task)
			assert.NoError(t, err)
			assert.True(t, isPatchAuthor)
		},
		"FalseWhenUserIsNotPatchAuthor": func(ctx context.Context, t *testing.T) {
			versionAndPatchID := mgobson.NewObjectId()
			patch := patch.Patch{
				Id:     versionAndPatchID,
				Author: "someone_else",
			}
			assert.NoError(t, patch.Insert(t.Context()))
			task := restModel.APITask{ProjectId: utility.ToStringPtr("random_project_id"), Version: utility.ToStringPtr(versionAndPatchID.Hex()), Requester: utility.ToStringPtr(evergreen.PatchVersionRequester)}
			isPatchAuthor, err := isPatchAuthorForTask(ctx, &task)
			assert.NoError(t, err)
			assert.False(t, isPatchAuthor)
		},
		"FalseWhenTaskRequesterIsNotPatchVersionRequester": func(ctx context.Context, t *testing.T) {
			versionAndPatchID := mgobson.NewObjectId()
			patch := patch.Patch{
				Id:     versionAndPatchID,
				Author: "basic_user",
			}
			assert.NoError(t, patch.Insert(t.Context()))
			task := restModel.APITask{ProjectId: utility.ToStringPtr("random_project_id"), Version: utility.ToStringPtr(versionAndPatchID.Hex()), Requester: utility.ToStringPtr(evergreen.TriggerRequester)}
			isPatchAuthor, err := isPatchAuthorForTask(ctx, &task)
			assert.NoError(t, err)
			assert.False(t, isPatchAuthor)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection, annotations.Collection, task.Collection, patch.Collection))
			usr := user.DBUser{
				Id: "basic_user",
			}
			assert.NoError(t, usr.Insert(t.Context()))
			ctx := loaders.Inject(gimlet.AttachUser(context.Background(), &usr))
			tCase(ctx, t)
		})
	}
}

func TestHasLogViewPermission(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, userWithRole gimlet.User, userWithoutRole gimlet.User){
		"TrueWhenUserHasRequiredPermission": func(ctx context.Context, t *testing.T, userWithRole gimlet.User, userWithoutRole gimlet.User) {
			ctx = gimlet.AttachUser(ctx, userWithRole)
			task := restModel.APITask{ProjectId: utility.ToStringPtr("project_id_belonging_to_user"), Version: utility.ToStringPtr("random_version_id")}
			hasAccess := hasLogViewPermission(ctx, &task)
			assert.True(t, hasAccess)
		},
		"FalseWhenUserDoesNotHaveRequiredPermission": func(ctx context.Context, t *testing.T, userWithRole gimlet.User, userWithoutRole gimlet.User) {
			ctx = gimlet.AttachUser(ctx, userWithoutRole)
			task := restModel.APITask{ProjectId: utility.ToStringPtr("project_id_belonging_to_user"), Version: utility.ToStringPtr("random_version_id")}
			hasAccess := hasLogViewPermission(ctx, &task)
			assert.False(t, hasAccess)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection, annotations.Collection, task.Collection, patch.Collection))
			userWithoutRole := user.DBUser{
				Id: "basic_user",
			}
			assert.NoError(t, userWithoutRole.Insert(t.Context()))
			userWithRole := user.DBUser{
				Id: "usr_with_log_view_role",
			}
			assert.NoError(t, userWithRole.Insert(t.Context()))
			ctx := gimlet.AttachUser(context.Background(), &userWithRole)
			env := evergreen.GetEnvironment()
			roleManager := env.RoleManager()
			projectScope := gimlet.Scope{
				ID:        "projectScopeID",
				Name:      "project scope",
				Type:      evergreen.ProjectResourceType,
				Resources: []string{"project_id_belonging_to_user"},
			}
			err := roleManager.AddScope(ctx, projectScope)

			logViewRole := gimlet.Role{
				ID:          "view_log",
				Scope:       projectScope.ID,
				Permissions: map[string]int{evergreen.PermissionLogs: evergreen.LogsView.Value},
			}
			require.NoError(t, roleManager.UpdateRole(ctx, logViewRole))

			require.NoError(t, userWithRole.AddRole(t.Context(), "view_log"))
			require.NoError(t, err)

			tCase(ctx, t, &userWithRole, &userWithoutRole)
		})
	}
}
func TestHasAnnotationPermission(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"TrueWhenUserHasRequiredLevelAndIsNotPatchOwner": func(ctx context.Context, t *testing.T) {
			task := restModel.APITask{ProjectId: utility.ToStringPtr("project_id_belonging_to_user"), Version: utility.ToStringPtr("random_version_id")}
			hasAccess, err := hasAnnotationPermission(ctx, &task, evergreen.AnnotationsView.Value)
			assert.NoError(t, err)
			assert.True(t, hasAccess)
		},
		"FalseWhenUserDoesNotHaveRequiredLevelAndIsNotPatchOwner": func(ctx context.Context, t *testing.T) {
			task := restModel.APITask{ProjectId: utility.ToStringPtr("project_id_belonging_to_user"), Version: utility.ToStringPtr("random_version_id")}
			hasAccess, err := hasAnnotationPermission(ctx, &task, evergreen.AnnotationsModify.Value)
			assert.NoError(t, err)
			assert.False(t, hasAccess)
		},
		"TrueWhenUserIsPatchOwnerButDoesNotHaveRequiredLevel": func(ctx context.Context, t *testing.T) {
			versionAndPatchID := mgobson.NewObjectId()
			patch := patch.Patch{
				Id:     versionAndPatchID,
				Author: "basic_user",
			}
			assert.NoError(t, patch.Insert(t.Context()))
			task := restModel.APITask{ProjectId: utility.ToStringPtr("random_project_id"), Version: utility.ToStringPtr(versionAndPatchID.Hex()), Requester: utility.ToStringPtr(evergreen.PatchVersionRequester)}
			hasAccess, err := hasAnnotationPermission(ctx, &task, evergreen.AnnotationsView.Value)
			assert.NoError(t, err)
			assert.True(t, hasAccess)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection, annotations.Collection, task.Collection, patch.Collection))
			usr := user.DBUser{
				Id: "basic_user",
			}
			assert.NoError(t, usr.Insert(t.Context()))
			ctx := loaders.Inject(gimlet.AttachUser(context.Background(), &usr))

			env := evergreen.GetEnvironment()
			roleManager := env.RoleManager()
			projectScope := gimlet.Scope{
				ID:        "projectScopeID",
				Name:      "project scope",
				Type:      evergreen.ProjectResourceType,
				Resources: []string{"project_id_belonging_to_user"},
			}
			err := roleManager.AddScope(ctx, projectScope)

			annotationViewRole := gimlet.Role{
				ID:          "view_annotation",
				Scope:       projectScope.ID,
				Permissions: map[string]int{evergreen.PermissionAnnotations: evergreen.AnnotationsView.Value},
			}
			require.NoError(t, roleManager.UpdateRole(ctx, annotationViewRole))

			require.NoError(t, usr.AddRole(t.Context(), "view_annotation"))
			require.NoError(t, err)

			tCase(ctx, t)
		})
	}
}

func TestFlattenOtelVariables(t *testing.T) {
	nestedVars := map[string]any{
		"k1": "v1",
		"k2": map[string]any{
			"nested_k3": "v3",
			"nested_k4": "v4",
		},
		"k5": "v5",
		"k6": map[string]any{
			"nested_k7": "v7",
		},
	}

	unnestedVars := flattenOtelVariables(nestedVars)
	assert.Len(t, unnestedVars, 5)

	val, ok := unnestedVars["k1"]
	assert.True(t, ok)
	assert.Equal(t, "v1", val)

	val, ok = unnestedVars["k5"]
	assert.True(t, ok)
	assert.Equal(t, "v5", val)

	val, ok = unnestedVars["k2.nested_k3"]
	assert.True(t, ok)
	assert.Equal(t, "v3", val)

	val, ok = unnestedVars["k2.nested_k4"]
	assert.True(t, ok)
	assert.Equal(t, "v4", val)

	val, ok = unnestedVars["k6.nested_k7"]
	assert.True(t, ok)
	assert.Equal(t, "v7", val)
}

func TestGetAPIProjectRef(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.ProjectRefCollection))

	projectRefContext := func(fields ...string) context.Context {
		selections := make(ast.SelectionSet, 0, len(fields))
		for _, field := range fields {
			selections = append(selections, &ast.Field{Name: field, Alias: field})
		}
		ctx := gqlgen.WithOperationContext(t.Context(), &gqlgen.OperationContext{})
		ctx = gqlgen.WithFieldContext(ctx, &gqlgen.FieldContext{
			Field: gqlgen.CollectedField{
				Field:      &ast.Field{Name: "projectRef", Alias: "projectRef"},
				Selections: selections,
			},
		})
		return loaders.Inject(ctx)
	}

	t.Run("ReturnsNilForDeletedProject", func(t *testing.T) {
		ctx := projectRefContext("owner")
		projectId := "deleted_project"
		result, err := getAPIProjectRef(ctx, &projectId)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("ReturnsProjectMetadataForExistingProject", func(t *testing.T) {
		ctx := projectRefContext("owner", "repo")
		projectRef := model.ProjectRef{
			Id:         "existing_project",
			Identifier: "existing_project",
			Owner:      "my_owner",
			Repo:       "my_repo",
			Branch:     "main",
		}
		assert.NoError(t, projectRef.Insert(ctx))
		projectId := "existing_project"
		result, err := getAPIProjectRef(ctx, &projectId)
		assert.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "my_owner", utility.FromStringPtr(result.Owner))
		assert.Equal(t, "my_repo", utility.FromStringPtr(result.Repo))
	})
}

func TestGetHostRequestOptionsDebugValidation(t *testing.T) {
	assert.NoError(t, db.ClearCollections(user.Collection, distro.Collection))

	usr := &user.DBUser{
		Id: "test_user",
	}
	assert.NoError(t, usr.Insert(t.Context()))

	d := &distro.Distro{
		Id:       "test-distro",
		Provider: evergreen.ProviderNameEc2Fleet,
	}
	assert.NoError(t, d.Insert(t.Context()))
	t.Run("IsDebugTrueWithoutSpawnHostsStartedByTaskFails", func(t *testing.T) {
		input := &SpawnHostInput{
			DistroID:                d.Id,
			IsDebug:                 utility.TruePtr(),
			SpawnHostsStartedByTask: utility.FalsePtr(),
			PublicKey: &PublicKeyInput{
				Name: "test-key",
				Key:  "ssh-rsa test",
			},
		}

		options, err := getHostRequestOptions(t.Context(), usr, input)
		require.Error(t, err)
		assert.Nil(t, options)
		assert.Contains(t, err.Error(), "Debug spawn hosts can only be spawned by a task")
	})

	t.Run("IsDebugFalseWorks", func(t *testing.T) {
		input := &SpawnHostInput{
			DistroID: d.Id,
			IsDebug:  utility.FalsePtr(),
			PublicKey: &PublicKeyInput{
				Name: "test-key",
				Key:  "ssh-rsa test",
			},
		}

		options, err := getHostRequestOptions(t.Context(), usr, input)
		assert.NoError(t, err)
		require.NotNil(t, options)
		assert.False(t, options.IsDebug, "IsDebug should be false")
	})

	t.Run("IsDebugNilDefaultsToFalse", func(t *testing.T) {
		input := &SpawnHostInput{
			DistroID: d.Id,
			PublicKey: &PublicKeyInput{
				Name: "test-key",
				Key:  "ssh-rsa test",
			},
		}

		options, err := getHostRequestOptions(t.Context(), usr, input)
		assert.NoError(t, err)
		require.NotNil(t, options)
		assert.False(t, options.IsDebug, "IsDebug should default to false")
	})
}

func TestGetHostRequestOptionsTaskValidation(t *testing.T) {
	setupPermissions(t)
	usr, err := setupUser(t)
	require.NoError(t, err)
	ctx := gimlet.AttachUser(t.Context(), usr)

	d := distro.Distro{Id: "test-distro"}
	require.NoError(t, d.Insert(ctx))
	tsk := task.Task{Id: "task-id", Project: "project_id"}
	require.NoError(t, tsk.Insert(ctx))

	t.Run("UnauthorizedTaskShouldRejectOptionsBeforeSavingKeyOrReplayingHosts", func(t *testing.T) {
		input := &SpawnHostInput{
			DistroID:                d.Id,
			TaskID:                  utility.ToStringPtr(tsk.Id),
			PublicKey:               &PublicKeyInput{Name: "test-key", Key: "ssh-rsa test"},
			SavePublicKey:           true,
			IsDebug:                 utility.TruePtr(),
			UseProjectSetupScript:   utility.TruePtr(),
			SpawnHostsStartedByTask: utility.TruePtr(),
		}
		options, err := getHostRequestOptions(ctx, usr, input)
		require.ErrorContains(t, err, "does not have permission to 'view tasks'")
		assert.Nil(t, options)
		storedUser, err := user.FindOneById(ctx, usr.Id)
		require.NoError(t, err)
		require.NotNil(t, storedUser)
		assert.Empty(t, storedUser.PubKeys)
	})

	t.Run("TaskViewPermissionShouldAllowDebugHostOptions", func(t *testing.T) {
		require.NoError(t, usr.AddRole(ctx, "view_task"))
		t.Cleanup(func() { require.NoError(t, usr.RemoveRole(ctx, "view_task")) })

		options, err := getHostRequestOptions(ctx, usr, &SpawnHostInput{
			DistroID:              d.Id,
			TaskID:                utility.ToStringPtr(tsk.Id),
			PublicKey:             &PublicKeyInput{Name: "test-key", Key: "ssh-rsa test"},
			IsDebug:               utility.TruePtr(),
			UseProjectSetupScript: utility.TruePtr(),
		})
		require.NoError(t, err)
		require.NotNil(t, options)
		assert.Equal(t, tsk.Id, options.TaskID)
		assert.True(t, options.IsDebug)
		assert.True(t, options.UseProjectSetupScript)
	})

	t.Run("EmptyTaskIDShouldNotRequireProjectPermission", func(t *testing.T) {
		options, err := getHostRequestOptions(ctx, usr, &SpawnHostInput{
			DistroID:  d.Id,
			TaskID:    utility.ToStringPtr(""),
			PublicKey: &PublicKeyInput{Name: "test-key", Key: "ssh-rsa test"},
		})
		require.NoError(t, err)
		require.NotNil(t, options)
		assert.Empty(t, options.TaskID)
	})

	input := &SpawnHostInput{TaskID: utility.ToStringPtr("missing-task")}

	t.Run("MissingTaskShouldReturnResourceNotFound", func(t *testing.T) {
		options, err := getHostRequestOptions(t.Context(), usr, input)
		require.EqualError(t, err, "input: task 'missing-task' not found")
		assert.Nil(t, options)
		var gqlErr *gqlerror.Error
		require.ErrorAs(t, err, &gqlErr)
		assert.Equal(t, ResourceNotFound, gqlErr.Extensions["code"])
	})

	t.Run("CanceledLookupShouldReturnInternalServerErrorWithCause", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		options, err := getHostRequestOptions(ctx, usr, input)
		require.Error(t, err)
		assert.Nil(t, options)
		assert.ErrorIs(t, err, context.Canceled)
		var gqlErr *gqlerror.Error
		require.ErrorAs(t, err, &gqlErr)
		assert.Equal(t, InternalServerError, gqlErr.Extensions["code"])
	})
}

func TestVerifyPublicKey(t *testing.T) {
	validKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIF8G5JYHL886OKVr2uzi1a9Cc5I2KdJCRc0sNjA7398C user@host"

	t.Run("AcceptsValidKey", func(t *testing.T) {
		err := verifyPublicKey(t.Context(), PublicKeyInput{Name: "key", Key: validKey})
		assert.NoError(t, err)
	})
	t.Run("RejectsExpansionSyntaxInComment", func(t *testing.T) {
		err := verifyPublicKey(t.Context(), PublicKeyInput{Name: "key", Key: fmt.Sprintf("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIF8G5JYHL886OKVr2uzi1a9Cc5I2KdJCRc0sNjA7398C ${%s}", evergreen.GithubAppPrivateKey)})
		require.Error(t, err)
	})
}

func TestResolveTaskSelection(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.VersionCollection, task.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(model.VersionCollection, task.Collection))
	})

	v := model.Version{
		Id:         "v1",
		Identifier: "proj",
		Requester:  evergreen.PatchVersionRequester,
	}
	require.NoError(t, v.Insert(t.Context()))

	makeTask := func(id, version, status string) task.Task {
		return task.Task{
			Id:                 id,
			DisplayName:        id,
			Version:            version,
			Project:            "proj",
			BuildVariant:       "bv1",
			Status:             status,
			DisplayStatusCache: status,
			ActivatedTime:      time.Now(),
			DisplayTaskId:      utility.ToStringPtr(""),
		}
	}
	tasks := []task.Task{
		makeTask("failed1", "v1", evergreen.TaskFailed),
		makeTask("failed2", "v1", evergreen.TaskFailed),
		makeTask("failed3", "v1", evergreen.TaskFailed),
		makeTask("succeeded1", "v1", evergreen.TaskSucceeded),
		makeTask("other_version", "v2", evergreen.TaskFailed),
	}
	for _, tsk := range tasks {
		require.NoError(t, tsk.Insert(t.Context()))
	}

	failedFilter := &TaskSelectionFilter{Statuses: []string{evergreen.TaskFailed}}

	t.Run("InvalidTaskSelection", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			versionID   string
			selection   TaskSelection
			expectedErr string
		}{
			"NeitherTaskIDsNorFilterErrors": {
				versionID:   "v1",
				selection:   TaskSelection{},
				expectedErr: "exactly one of 'taskIds' or 'filter' must be set in a task selection",
			},
			"BothTaskIDsAndFilterErrors": {
				versionID:   "v1",
				selection:   TaskSelection{TaskIds: []string{"failed1"}, Filter: failedFilter},
				expectedErr: "exactly one of 'taskIds' or 'filter' must be set in a task selection",
			},
			"FilterOnUnknownVersionErrors": {
				versionID:   "nonexistent",
				selection:   TaskSelection{Filter: failedFilter},
				expectedErr: "version 'nonexistent' not found",
			},
		} {
			t.Run(testName, func(t *testing.T) {
				_, _, err := resolveTaskSelection(t.Context(), testCase.versionID, testCase.selection)
				require.Error(t, err)
				assert.ErrorContains(t, err, testCase.expectedErr)
			})
		}
	})

	t.Run("TaskSelectionWithTaskIds", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			taskIDs          []string
			expectedIDs      []string
			expectedErrorIDs []string
		}{
			"ReturnsOnlyTheRequestedTasks": {
				taskIDs:     []string{"failed1", "succeeded1"},
				expectedIDs: []string{"failed1", "succeeded1"},
			},
			"TasksInAnotherVersionAreReportedAsTaskErrors": {
				taskIDs:          []string{"failed1", "other_version"},
				expectedIDs:      []string{"failed1"},
				expectedErrorIDs: []string{"other_version"},
			},
			"NonexistentTasksAreReportedAsTaskErrorsOnce": {
				taskIDs:          []string{"nonexistent", "nonexistent"},
				expectedErrorIDs: []string{"nonexistent"},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				resolved, taskErrors, err := resolveTaskSelection(t.Context(), "v1", TaskSelection{TaskIds: testCase.taskIDs})
				require.NoError(t, err)
				taskIDs := make([]string, 0, len(resolved))
				for _, t := range resolved {
					taskIDs = append(taskIDs, t.Id)
				}
				assert.ElementsMatch(t, testCase.expectedIDs, taskIDs)
				errorIDs := []string{}
				for _, e := range taskErrors {
					errorIDs = append(errorIDs, e.TaskID)
				}
				assert.ElementsMatch(t, testCase.expectedErrorIDs, errorIDs)
			})
		}
	})

	t.Run("TaskSelectionWithFilter", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			filter      *TaskSelectionFilter
			expectedIDs []string
		}{
			"ReturnsAllMatchingTasksInVersion": {
				filter:      failedFilter,
				expectedIDs: []string{"failed1", "failed2", "failed3"},
			},
			"EmptyFilterReturnsAllTasksInVersion": {
				filter:      &TaskSelectionFilter{},
				expectedIDs: []string{"failed1", "failed2", "failed3", "succeeded1"},
			},
			"OmitsExcludedTasks": {
				filter: &TaskSelectionFilter{
					Statuses:       []string{evergreen.TaskFailed},
					ExcludeTaskIds: []string{"failed2"},
				},
				expectedIDs: []string{"failed1", "failed3"},
			},
			"MatchingNothingReturnsNoTasks": {
				filter: &TaskSelectionFilter{Statuses: []string{evergreen.TaskStarted}},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				resolved, taskErrors, err := resolveTaskSelection(t.Context(), "v1", TaskSelection{Filter: testCase.filter})
				require.NoError(t, err)
				taskIDs := make([]string, 0, len(resolved))
				for _, t := range resolved {
					taskIDs = append(taskIDs, t.Id)
				}
				assert.ElementsMatch(t, testCase.expectedIDs, taskIDs)
				assert.Empty(t, taskErrors)
			})
		}
	})
}

func TestGetSchedulableTaskIDs(t *testing.T) {
	unscheduled := task.Task{Id: "unscheduled", Status: evergreen.TaskUndispatched, Activated: false}
	scheduled := task.Task{Id: "scheduled", Status: evergreen.TaskUndispatched, Activated: true}
	running := task.Task{Id: "running", Status: evergreen.TaskStarted, Activated: true}
	finished := task.Task{Id: "finished", Status: evergreen.TaskFailed, Activated: true}
	mergeQueue := task.Task{Id: "mergeQueue", Status: evergreen.TaskUndispatched, Activated: false, Requester: evergreen.GithubMergeRequester}
	unscheduledDisplay := task.Task{Id: "display", DisplayOnly: true, Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("")}
	scheduledDisplay := task.Task{Id: "scheduledDisplay", DisplayOnly: true, Status: evergreen.TaskUndispatched, Activated: true, DisplayTaskId: utility.ToStringPtr("")}
	unscheduledExec1 := task.Task{Id: "exec1", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("display")}
	unscheduledExec2 := task.Task{Id: "exec2", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("display")}
	scheduledExec := task.Task{Id: "scheduledExec", Status: evergreen.TaskUndispatched, Activated: true, DisplayTaskId: utility.ToStringPtr("display")}

	for testName, testCase := range map[string]struct {
		tasks            []task.Task
		expectedIDs      []string
		expectedErrorIDs []string
	}{
		"UnscheduledTaskIsSchedulable": {
			tasks:       []task.Task{unscheduled},
			expectedIDs: []string{"unscheduled"},
		},
		"ScheduledRunningAndFinishedTasksAreNotSchedulable": {
			tasks:            []task.Task{scheduled, running, finished},
			expectedErrorIDs: []string{"scheduled", "running", "finished"},
		},
		"UnscheduledMergeQueueTaskIsNotSchedulable": {
			tasks:            []task.Task{mergeQueue},
			expectedErrorIDs: []string{"mergeQueue"},
		},
		"UnscheduledDisplayTaskIsSchedulable": {
			tasks:       []task.Task{unscheduledDisplay},
			expectedIDs: []string{"display"},
		},
		"ScheduledDisplayTaskIsNotSchedulable": {
			tasks:            []task.Task{scheduledDisplay},
			expectedErrorIDs: []string{"scheduledDisplay"},
		},
		"UnscheduledExecutionTaskIsReplacedByItsDisplayTask": {
			tasks:       []task.Task{unscheduledExec1},
			expectedIDs: []string{"display"},
		},
		"SiblingExecutionTasksCollapseToOneDisplayTask": {
			tasks:       []task.Task{unscheduledExec1, unscheduledExec2},
			expectedIDs: []string{"display"},
		},
		"DisplayTaskAlreadySelectedIsNotDuplicated": {
			tasks:       []task.Task{unscheduledDisplay, unscheduledExec1},
			expectedIDs: []string{"display"},
		},
		"ScheduledExecutionTaskIsNotSchedulable": {
			tasks:            []task.Task{scheduledExec},
			expectedErrorIDs: []string{"scheduledExec"},
		},
		"MixedTasksAreSplitBySchedulability": {
			tasks:            []task.Task{unscheduled, finished, unscheduledDisplay, scheduledDisplay},
			expectedIDs:      []string{"unscheduled", "display"},
			expectedErrorIDs: []string{"finished", "scheduledDisplay"},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ids, taskErrors := getSchedulableTaskIDs(testCase.tasks)
			errorIDs := []string{}
			for _, e := range taskErrors {
				errorIDs = append(errorIDs, e.TaskID)
			}
			assert.ElementsMatch(t, testCase.expectedIDs, ids)
			assert.ElementsMatch(t, testCase.expectedErrorIDs, errorIDs)
		})
	}
}
