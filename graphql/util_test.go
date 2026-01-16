package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	status, err := getDisplayStatus(t.Context(), version)
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
			ctx := gimlet.AttachUser(context.Background(), &usr)
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
			ctx := gimlet.AttachUser(context.Background(), &usr)

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

func TestGetHostRequestOptionsDebugValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(user.Collection, distro.Collection))

	usr := &user.DBUser{
		Id: "test_user",
	}
	assert.NoError(t, usr.Insert(ctx))

	d := &distro.Distro{
		Id:       "test-distro",
		Provider: evergreen.ProviderNameEc2OnDemand,
	}
	assert.NoError(t, d.Insert(ctx))
	
	t.Run("IsDebugTrueWithoutSpawnHostsStartedByTaskFails", func(t *testing.T) {
		input := &SpawnHostInput{
			DistroID:                "test-distro",
			IsDebug:                 utility.ToBoolPtr(true),
			SpawnHostsStartedByTask: utility.ToBoolPtr(false),
			PublicKey: &PublicKeyInput{
				Name: "test-key",
				Key:  "ssh-rsa test",
			},
			Region:               "us-east-1",
			IsVirtualWorkStation: false,
			NoExpiration:         false,
		}

		options, err := getHostRequestOptions(ctx, usr, input)
		assert.Error(t, err)
		assert.Nil(t, options)
		assert.Contains(t, err.Error(), "Debug spawn hosts can only be spawned by a task")
	})

	t.Run("IsDebugFalseWorks", func(t *testing.T) {
		input := &SpawnHostInput{
			DistroID: "test-distro",
			IsDebug:  utility.ToBoolPtr(false),
			PublicKey: &PublicKeyInput{
				Name: "test-key",
				Key:  "ssh-rsa test",
			},
			Region:               "us-east-1",
			IsVirtualWorkStation: false,
			NoExpiration:         false,
		}

		options, err := getHostRequestOptions(ctx, usr, input)
		assert.NoError(t, err)
		assert.NotNil(t, options)
		assert.False(t, options.IsDebug, "IsDebug should be false")
	})

	t.Run("IsDebugNilDefaultsToFalse", func(t *testing.T) {
		input := &SpawnHostInput{
			DistroID: "test-distro",
			PublicKey: &PublicKeyInput{
				Name: "test-key",
				Key:  "ssh-rsa test",
			},
			Region:               "us-east-1",
			IsVirtualWorkStation: false,
			NoExpiration:         false,
		}

		options, err := getHostRequestOptions(ctx, usr, input)
		assert.NoError(t, err)
		assert.NotNil(t, options)
		assert.False(t, options.IsDebug, "IsDebug should default to false")
	})
}
