package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

func TestFindOneProjectRef(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require.NoError(t, db.Clear(ProjectRefCollection),
		"Error clearing collection")
	projectRef := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "main",
		Enabled:   utility.TruePtr(),
		BatchTime: 10,
		Id:        "ident",
	}
	assert.Nil(projectRef.Insert())

	projectRefFromDB, err := FindOneProjectRef("ident")
	assert.Nil(err)
	assert.NotNil(projectRefFromDB)

	assert.Equal(projectRefFromDB.Owner, "mongodb")
	assert.Equal(projectRefFromDB.Repo, "mci")
	assert.Equal(projectRefFromDB.Branch, "main")
	assert.True(projectRefFromDB.IsEnabled())
	assert.Equal(projectRefFromDB.BatchTime, 10)
	assert.Equal(projectRefFromDB.Id, "ident")
	assert.Equal(projectRefFromDB.DefaultLogger, "buildlogger")
}

func TestFindMergedProjectRef(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection),
		"Error clearing collection")

	projectRef := &ProjectRef{
		Owner:                 "mongodb",
		RepoRefId:             "mongodb_mci",
		BatchTime:             10,
		Id:                    "ident",
		Admins:                []string{"john.smith", "john.doe"},
		UseRepoSettings:       true,
		Enabled:               utility.FalsePtr(),
		PatchingDisabled:      utility.FalsePtr(),
		RepotrackerDisabled:   utility.TruePtr(),
		PRTestingEnabled:      nil,
		GitTagVersionsEnabled: nil,
		GitTagAuthorizedTeams: []string{},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{ChildProject: "a different branch"},
		},
		CommitQueue:       CommitQueueParams{Enabled: nil, Message: "using repo commit queue"},
		WorkstationConfig: WorkstationConfig{GitClone: true},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.FalsePtr()},
	}
	assert.NoError(t, projectRef.Insert())
	repoRef := &RepoRef{ProjectRef{
		Id:                    "mongodb_mci",
		Repo:                  "mci",
		Branch:                "main",
		SpawnHostScriptPath:   "my-path",
		Admins:                []string{"john.liu"},
		Enabled:               utility.TruePtr(),
		PatchingDisabled:      nil,
		GitTagVersionsEnabled: utility.FalsePtr(),
		PRTestingEnabled:      utility.TruePtr(),
		GitTagAuthorizedTeams: []string{"my team"},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{Alias: "global patch trigger"},
		},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.TruePtr(), PatchEnabled: utility.TruePtr()},
		CommitQueue:       CommitQueueParams{Enabled: utility.TruePtr()},
		WorkstationConfig: WorkstationConfig{SetupCommands: []WorkstationSetupCommand{{Command: "my-command"}}},
	}}
	assert.NoError(t, repoRef.Insert())

	mergedProject, err := FindMergedProjectRef("ident")
	assert.NoError(t, err)
	require.NotNil(t, mergedProject)
	assert.Equal(t, "ident", mergedProject.Id)
	require.Len(t, mergedProject.Admins, 2)
	assert.Contains(t, mergedProject.Admins, "john.smith")
	assert.Contains(t, mergedProject.Admins, "john.doe")
	assert.NotContains(t, mergedProject.Admins, "john.liu")
	assert.False(t, *mergedProject.Enabled)
	assert.False(t, mergedProject.IsPatchingDisabled())
	assert.True(t, mergedProject.UseRepoSettings)
	assert.True(t, mergedProject.IsRepotrackerDisabled())
	assert.False(t, mergedProject.IsGitTagVersionsEnabled())
	assert.False(t, mergedProject.IsGithubChecksEnabled())
	assert.True(t, mergedProject.IsPRTestingEnabled())
	assert.Equal(t, "my-path", mergedProject.SpawnHostScriptPath)
	assert.False(t, utility.FromBoolPtr(mergedProject.TaskSync.ConfigEnabled))
	assert.True(t, utility.FromBoolPtr(mergedProject.TaskSync.PatchEnabled))
	assert.Len(t, mergedProject.GitTagAuthorizedTeams, 1)
	require.Len(t, mergedProject.PatchTriggerAliases, 1)
	assert.Empty(t, mergedProject.PatchTriggerAliases[0].Alias)
	assert.Equal(t, "a different branch", mergedProject.PatchTriggerAliases[0].ChildProject)

	assert.True(t, mergedProject.CommitQueue.IsEnabled())
	assert.Equal(t, "using repo commit queue", mergedProject.CommitQueue.Message)

	assert.True(t, mergedProject.WorkstationConfig.GitClone)
	assert.Len(t, mergedProject.WorkstationConfig.SetupCommands, 1)
}

func TestGetBatchTimeDoesNotExceedMaxBatchTime(t *testing.T) {
	assert := assert.New(t)

	projectRef := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "main",
		Enabled:   utility.TruePtr(),
		BatchTime: maxBatchTime + 1,
		Id:        "ident",
	}

	emptyVariant := &BuildVariant{}
	emptyTask := &BuildVariantTaskUnit{}

	assert.Equal(projectRef.getBatchTimeForVariant(emptyVariant), maxBatchTime,
		"ProjectRef.getBatchTimeForVariant() is not capping BatchTime to MaxInt32")

	assert.Equal(projectRef.getBatchTimeForTask(emptyTask), maxBatchTime,
		"ProjectRef.getBatchTimeForTask() is not capping BatchTime to MaxInt32")

	projectRef.BatchTime = 55
	assert.Equal(projectRef.getBatchTimeForVariant(emptyVariant), 55,
		"ProjectRef.getBatchTimeForVariant() is not returning the correct BatchTime")

	assert.Equal(projectRef.getBatchTimeForTask(emptyTask), 55,
		"ProjectRef.getBatchTimeForVariant() is not returning the correct BatchTime")

}

func TestGetActivationTimeForTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection))
	prevTime := time.Date(2020, time.June, 9, 0, 0, 0, 0, time.UTC) // Tuesday
	batchTime := 60
	projectRef := &ProjectRef{Id: "mci"}
	bvt := &BuildVariantTaskUnit{
		BatchTime: &batchTime,
		Name:      "myTask",
		Variant:   "bv1",
	}

	versionWithoutTask := Version{
		Id:         "v1",
		Identifier: projectRef.Id,
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant:     "bv1",
				ActivationStatus: ActivationStatus{Activated: true, ActivateAt: time.Now()},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskName:         "a different task",
						ActivationStatus: ActivationStatus{ActivateAt: time.Now(), Activated: true},
					},
				},
			},
		},
	}
	versionWithTask := Version{
		Id:         "v2",
		Identifier: projectRef.Id,
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant:     "bv1",
				ActivationStatus: ActivationStatus{Activated: true, ActivateAt: prevTime.Add(-1 * time.Hour)},
				BatchTimeTasks: []BatchTimeTaskStatus{
					{
						TaskName:         "myTask",
						ActivationStatus: ActivationStatus{ActivateAt: prevTime, Activated: true},
					},
					{
						TaskName:         "notMyTask",
						ActivationStatus: ActivationStatus{ActivateAt: time.Now(), Activated: true},
					},
				},
			},
			{
				BuildVariant:     "bv_unrelated",
				ActivationStatus: ActivationStatus{Activated: true, ActivateAt: time.Now()},
			},
		},
	}
	assert.NoError(t, versionWithoutTask.Insert())
	assert.NoError(t, versionWithTask.Insert())

	activationTime, err := projectRef.GetActivationTimeForTask(bvt)
	assert.NoError(t, err)
	assert.True(t, activationTime.Equal(prevTime.Add(time.Hour)))
}

func TestGetActivationTimeWithCron(t *testing.T) {
	prevTime := time.Date(2020, time.June, 9, 0, 0, 0, 0, time.UTC) // Tuesday
	for name, test := range map[string]func(t *testing.T){
		"Empty": func(t *testing.T) {
			_, err := GetActivationTimeWithCron(prevTime, "")
			assert.Error(t, err)
		},
		"InvalidBatchSyntax": func(t *testing.T) {
			batchStr := "* * *"
			_, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.Error(t, err)
		},
		"EveryHourEveryDay": func(t *testing.T) {
			batchStr := "0 * * * *"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour), res)
		},
		"SpecifyDOW": func(t *testing.T) {
			batchStr := "0 0 ? * MON,WED,FRI"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24), res) // i.e. Wednesday

			newRes, err := GetActivationTimeWithCron(res, batchStr) // i.e. Friday
			assert.NoError(t, err)
			assert.Equal(t, res.Add(time.Hour*48), newRes)
		},
		"1and15thOfTheMonth": func(t *testing.T) {
			batchStr := "0 0 1,15 *"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24*6), res)
		},
		"Descriptor": func(t *testing.T) {
			batchStr := "@daily"
			res, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24), res)
		},
		"Interval": func(t *testing.T) {
			batchStr := "@every 2h"
			_, err := GetActivationTimeWithCron(prevTime, batchStr)
			assert.Error(t, err)
		},
	} {
		t.Run(name, test)
	}
}

func TestFindProjectRefsByRepoAndBranch(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require := require.New(t)

	assert.NoError(db.Clear(ProjectRefCollection))

	projectRefs, err := FindMergedProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "main",
		Enabled:          utility.FalsePtr(),
		BatchTime:        10,
		Id:               "iden_",
		PRTestingEnabled: utility.TruePtr(),
	}
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindMergedProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef.Id = "ident"
	projectRef.Enabled = utility.TruePtr()
	assert.NoError(projectRef.Insert())

	projectRefs, err = FindMergedProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	require.Len(projectRefs, 1)
	assert.Equal("ident", projectRefs[0].Id)
	assert.Equal("buildlogger", projectRefs[0].DefaultLogger)

	projectRef.Id = "ident2"
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindMergedProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Len(projectRefs, 2)
}

func TestFindOneProjectRefByRepoAndBranchWithPRTesting(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)   //nolint
	require := require.New(t) //nolint

	require.NoError(db.Clear(ProjectRefCollection))

	projectRef, err := FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "main",
		Enabled:          utility.FalsePtr(),
		BatchTime:        10,
		Id:               "ident0",
		PRTestingEnabled: utility.FalsePtr(),
	}
	require.NoError(doc.Insert())

	// 1 disabled document = no match
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	// 2 docs, 1 enabled, but the enabled one has pr testing disabled = no match
	doc.Id = "ident_"
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.Enabled = utility.TruePtr()
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	require.Nil(projectRef)

	// 3 docs, 2 enabled, but only 1 has pr testing enabled = match
	doc.Id = "ident1"
	doc.PRTestingEnabled = utility.TruePtr()
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("ident1", projectRef.Id)
	assert.Equal("buildlogger", projectRef.DefaultLogger)

	// 2 matching documents, error!
	doc.Id = "ident2"
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main")
	assert.Error(err)
	assert.Contains(err.Error(), "found 2 project refs, when 1 was expected")
	require.Nil(projectRef)
}

func TestFindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.Clear(ProjectRefCollection))

	projectRef, err := FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "mci",
		Enabled: utility.TruePtr(),
	}
	require.NoError(doc.Insert())

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc.CommitQueue.Enabled = utility.TruePtr()
	require.NoError(db.Update(ProjectRefCollection, bson.M{ProjectRefIdKey: "mci"}, doc))

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.NotNil(projectRef)
	assert.Equal("mci", projectRef.Id)
	assert.Equal("buildlogger", projectRef.DefaultLogger)
}

func TestCanEnableCommitQueue(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.Clear(ProjectRefCollection))
	doc := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "mci",
		Enabled: utility.TruePtr(),
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	require.NoError(doc.Insert())
	ok, err := doc.CanEnableCommitQueue()
	assert.NoError(err)
	assert.True(ok)

	doc2 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "not-mci",
		Enabled: utility.TruePtr(),
		CommitQueue: CommitQueueParams{
			Enabled: utility.FalsePtr(),
		},
	}
	require.NoError(doc2.Insert())
	ok, err = doc2.CanEnableCommitQueue()
	assert.NoError(err)
	assert.False(ok)
}

func TestFindProjectRefsWithCommitQueueEnabled(t *testing.T) {
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.Clear(ProjectRefCollection))
	projectRefs, err := FindProjectRefsWithCommitQueueEnabled()
	assert.NoError(err)
	assert.Empty(projectRefs)

	doc := &ProjectRef{
		Enabled:    utility.TruePtr(),
		Owner:      "mongodb",
		Repo:       "mci",
		Branch:     "main",
		Identifier: "mci",
		Id:         "1",
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	require.NoError(doc.Insert())

	doc.Branch = "fix"
	doc.Id = "2"
	require.NoError(doc.Insert())

	doc.Identifier = "grip"
	doc.Repo = "grip"
	doc.Id = "3"
	doc.CommitQueue.Enabled = utility.FalsePtr()
	require.NoError(doc.Insert())

	projectRefs, err = FindProjectRefsWithCommitQueueEnabled()
	assert.NoError(err)
	require.Len(projectRefs, 2)
	assert.Equal("mci", projectRefs[0].Identifier)
	assert.Equal("buildlogger", projectRefs[0].DefaultLogger)
	assert.Equal("mci", projectRefs[1].Identifier)
	assert.Equal("buildlogger", projectRefs[1].DefaultLogger)
}

func TestValidatePeriodicBuildDefinition(t *testing.T) {
	assert := assert.New(t)
	testCases := map[PeriodicBuildDefinition]bool{
		PeriodicBuildDefinition{
			IntervalHours: 24,
			ConfigFile:    "foo.yml",
			Alias:         "myAlias",
		}: true,
		PeriodicBuildDefinition{
			IntervalHours: 0,
			ConfigFile:    "foo.yml",
			Alias:         "myAlias",
		}: false,
		PeriodicBuildDefinition{
			IntervalHours: 24,
			ConfigFile:    "",
			Alias:         "myAlias",
		}: false,
		PeriodicBuildDefinition{
			IntervalHours: 24,
			ConfigFile:    "foo.yml",
			Alias:         "",
		}: false,
	}

	for testCase, shouldPass := range testCases {
		if shouldPass {
			assert.NoError(testCase.Validate())
		} else {
			assert.Error(testCase.Validate())
		}
		assert.NotEmpty(testCase.ID)
	}
}

func TestGetPatchTriggerAlias(t *testing.T) {
	projRef := ProjectRef{
		PatchTriggerAliases: []patch.PatchTriggerDefinition{{Alias: "a0"}},
	}

	alias, found := projRef.GetPatchTriggerAlias("a0")
	assert.True(t, found)
	assert.Equal(t, "a0", alias.Alias)

	alias, found = projRef.GetPatchTriggerAlias("a1")
	assert.False(t, found)
}

func TestFindDownstreamProjects(t *testing.T) {
	require.NoError(t, db.Clear(ProjectRefCollection))
	evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger = "buildlogger"

	proj1 := ProjectRef{
		Id:       "evergreen",
		Enabled:  utility.TruePtr(),
		Triggers: []TriggerDefinition{{Project: "grip"}},
	}
	require.NoError(t, proj1.Insert())

	proj2 := ProjectRef{
		Id:       "mci",
		Enabled:  utility.FalsePtr(),
		Triggers: []TriggerDefinition{{Project: "grip"}},
	}
	require.NoError(t, proj2.Insert())

	projects, err := FindDownstreamProjects("grip")
	assert.NoError(t, err)
	assert.Len(t, projects, 1)
	proj1.DefaultLogger = "buildlogger"
	assert.Equal(t, proj1, projects[0])
}

func TestAddPermissions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(user.Collection, ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	u := user.DBUser{
		Id: "me",
	}
	assert.NoError(u.Insert())
	p := ProjectRef{
		Identifier: "myProject",
	}
	assert.NoError(p.Add(&u))
	assert.NotEmpty(p.Id)
	assert.True(mgobson.IsObjectIdHex(p.Id))

	rm := evergreen.GetEnvironment().RoleManager()
	scope, err := rm.FindScopeForResources(evergreen.ProjectResourceType, p.Id)
	assert.NoError(err)
	assert.NotNil(scope)
	role, err := rm.FindRoleWithPermissions(evergreen.ProjectResourceType, []string{p.Id}, map[string]int{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
		evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
		evergreen.PermissionLogs:            evergreen.LogsView.Value,
	})
	assert.NoError(err)
	assert.NotNil(role)
	dbUser, err := user.FindOneById(u.Id)
	assert.NoError(err)
	assert.Contains(dbUser.Roles(), fmt.Sprintf("admin_project_%s", p.Id))
}

func TestUpdateAdminRoles(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection))
	env := evergreen.GetEnvironment()
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	rm := env.RoleManager()
	adminScope := gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"proj"},
	}
	require.NoError(t, rm.AddScope(adminScope))
	adminRole := gimlet.Role{
		ID:          "admin",
		Scope:       evergreen.AllProjectsScope,
		Permissions: adminPermissions,
	}
	require.NoError(t, rm.UpdateRole(adminRole))
	oldAdmin := user.DBUser{
		Id:          "oldAdmin",
		SystemRoles: []string{"admin"},
	}
	require.NoError(t, oldAdmin.Insert())
	newAdmin := user.DBUser{
		Id: "newAdmin",
	}
	require.NoError(t, newAdmin.Insert())
	p := ProjectRef{
		Id: "proj",
	}
	require.NoError(t, p.Insert())

	assert.NoError(t, p.UpdateAdminRoles([]string{newAdmin.Id}, []string{oldAdmin.Id}))
	oldAdminFromDB, err := user.FindOneById(oldAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, oldAdminFromDB.Roles(), 0)
	newAdminFromDB, err := user.FindOneById(newAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, newAdminFromDB.Roles(), 1)
}

func TestUpdateNextPeriodicBuild(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(ProjectRefCollection))
	now := time.Now().Truncate(time.Second)
	p := ProjectRef{
		Id: "proj",
		PeriodicBuilds: []PeriodicBuildDefinition{
			{ID: "1", NextRunTime: now},
			{ID: "2", NextRunTime: now.Add(1 * time.Hour)},
		},
	}
	assert.NoError(p.Insert())

	assert.NoError(p.UpdateNextPeriodicBuild("2", now.Add(10*time.Hour)))
	dbProject, err := FindOneProjectRef(p.Id)
	assert.NoError(err)
	assert.True(now.Equal(dbProject.PeriodicBuilds[0].NextRunTime))
	assert.True(now.Equal(p.PeriodicBuilds[0].NextRunTime))
	assert.True(now.Add(10 * time.Hour).Equal(dbProject.PeriodicBuilds[1].NextRunTime))
	assert.True(now.Add(10 * time.Hour).Equal(p.PeriodicBuilds[1].NextRunTime))
}

func TestGetProjectSetupCommands(t *testing.T) {
	p := ProjectRef{}
	p.WorkstationConfig.SetupCommands = []WorkstationSetupCommand{
		{Command: "c0"},
		{Command: "c1"},
	}

	cmds, err := p.GetProjectSetupCommands(apimodels.WorkstationSetupCommandOptions{})
	assert.NoError(t, err)
	assert.Len(t, cmds, 2)
	assert.Contains(t, cmds[0].String(), "c0")
	assert.Contains(t, cmds[1].String(), "c1")
}

func TestPointers(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection))
	ref := struct {
		MyString string            `bson:"my_str"`
		MyBool   bool              `bson:"my_bool"`
		MyStruct WorkstationConfig `bson:"config"`
	}{
		MyString: "this is a string",
		MyBool:   false,
		MyStruct: WorkstationConfig{GitClone: true},
	}

	assert.NoError(t, db.Insert(ProjectRefCollection, ref))

	pointerRef := struct {
		PtrString *string            `bson:"my_str"`
		PtrBool   *bool              `bson:"my_bool"`
		PtrStruct *WorkstationConfig `bson:"config"`
	}{}
	assert.NoError(t, db.FindOne(ProjectRefCollection, nil, nil, nil, &pointerRef))
	assert.Equal(t, ref.MyString, *pointerRef.PtrString)
	assert.False(t, utility.FromBoolTPtr(pointerRef.PtrBool))
	assert.NotNil(t, pointerRef.PtrStruct)
	assert.True(t, pointerRef.PtrStruct.GitClone)
}
