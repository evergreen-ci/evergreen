package model

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	adb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestFindOneProjectRef(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(ProjectRefCollection))
	projectRef := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "main",
		Enabled:   true,
		BatchTime: 10,
		Id:        "ident",
	}
	assert.Nil(projectRef.Insert())

	projectRefFromDB, err := FindBranchProjectRef("ident")
	assert.Nil(err)
	assert.NotNil(projectRefFromDB)

	assert.Equal(projectRefFromDB.Owner, "mongodb")
	assert.Equal(projectRefFromDB.Repo, "mci")
	assert.Equal(projectRefFromDB.Branch, "main")
	assert.True(projectRefFromDB.Enabled)
	assert.Equal(projectRefFromDB.BatchTime, 10)
	assert.Equal(projectRefFromDB.Id, "ident")
}

func TestFindMergedProjectRef(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ParserProjectCollection, ProjectConfigCollection))

	projectConfig := &ProjectConfig{
		Id: "ident",
		ProjectConfigFields: ProjectConfigFields{
			TaskAnnotationSettings: &evergreen.AnnotationsSettings{
				FileTicketWebhook: evergreen.WebHook{
					Endpoint: "random2",
				},
			},
		},
	}
	assert.NoError(t, projectConfig.Insert())

	projectRef := &ProjectRef{
		Owner:                 "mongodb",
		RepoRefId:             "mongodb_mci",
		BatchTime:             10,
		Id:                    "ident",
		Admins:                []string{"john.smith", "john.doe"},
		Enabled:               false,
		PatchingDisabled:      utility.FalsePtr(),
		RepotrackerDisabled:   utility.TruePtr(),
		DeactivatePrevious:    utility.TruePtr(),
		VersionControlEnabled: utility.TruePtr(),
		PRTestingEnabled:      nil,
		GitTagVersionsEnabled: nil,
		GitTagAuthorizedTeams: []string{},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{ChildProject: "a different branch"},
		},
		CommitQueue:       CommitQueueParams{Enabled: nil, Message: "using repo commit queue"},
		WorkstationConfig: WorkstationConfig{GitClone: utility.TruePtr()},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.FalsePtr()},
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "project-filter",
				CaseSensitive: true,
				ExactMatch:    false,
			},
		},
	}
	assert.NoError(t, projectRef.Insert())
	repoRef := &RepoRef{ProjectRef{
		Id:                    "mongodb_mci",
		Repo:                  "mci",
		Branch:                "main",
		SpawnHostScriptPath:   "my-path",
		Admins:                []string{"john.liu"},
		PatchingDisabled:      nil,
		GitTagVersionsEnabled: utility.FalsePtr(),
		PRTestingEnabled:      utility.TruePtr(),
		GitTagAuthorizedTeams: []string{"my team"},
		GitTagAuthorizedUsers: []string{"my user"},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{Alias: "global patch trigger"},
		},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.TruePtr(), PatchEnabled: utility.TruePtr()},
		CommitQueue:       CommitQueueParams{Enabled: utility.TruePtr()},
		WorkstationConfig: WorkstationConfig{SetupCommands: []WorkstationSetupCommand{{Command: "my-command"}}},
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "repo-filter",
				CaseSensitive: false,
				ExactMatch:    true,
			},
		},
	}}
	assert.NoError(t, repoRef.Upsert())

	mergedProject, err := FindMergedProjectRef("ident", "ident", true)
	assert.NoError(t, err)
	require.NotNil(t, mergedProject)
	assert.Equal(t, "ident", mergedProject.Id)
	require.Len(t, mergedProject.Admins, 2)
	assert.Contains(t, mergedProject.Admins, "john.smith")
	assert.Contains(t, mergedProject.Admins, "john.doe")
	assert.NotContains(t, mergedProject.Admins, "john.liu")
	assert.False(t, mergedProject.Enabled)
	assert.False(t, mergedProject.IsPatchingDisabled())
	assert.True(t, mergedProject.UseRepoSettings())
	assert.True(t, mergedProject.IsRepotrackerDisabled())
	assert.False(t, mergedProject.IsGitTagVersionsEnabled())
	assert.False(t, mergedProject.IsGithubChecksEnabled())
	assert.True(t, mergedProject.IsPRTestingEnabled())
	assert.Equal(t, "my-path", mergedProject.SpawnHostScriptPath)
	assert.False(t, utility.FromBoolPtr(mergedProject.TaskSync.ConfigEnabled))
	assert.True(t, utility.FromBoolPtr(mergedProject.TaskSync.PatchEnabled))
	assert.Len(t, mergedProject.GitTagAuthorizedTeams, 0) // empty lists take precedent
	assert.Len(t, mergedProject.GitTagAuthorizedUsers, 1)
	require.Len(t, mergedProject.PatchTriggerAliases, 1)
	assert.Empty(t, mergedProject.PatchTriggerAliases[0].Alias)
	assert.Equal(t, "a different branch", mergedProject.PatchTriggerAliases[0].ChildProject)

	assert.True(t, mergedProject.CommitQueue.IsEnabled())
	assert.Equal(t, "using repo commit queue", mergedProject.CommitQueue.Message)

	assert.True(t, mergedProject.WorkstationConfig.ShouldGitClone())
	assert.Len(t, mergedProject.WorkstationConfig.SetupCommands, 1)
	assert.Equal(t, "random2", mergedProject.TaskAnnotationSettings.FileTicketWebhook.Endpoint)
	assert.Len(t, mergedProject.ParsleyFilters, 2)

	// Assert that mergeParsleyFilters correctly handles projects with repo filters but not project filters.
	projectRef.ParsleyFilters = []parsley.Filter{}

	assert.NoError(t, projectRef.Upsert())
	mergedProject, err = FindMergedProjectRef("ident", "ident", true)
	assert.NoError(t, err)
	assert.Len(t, mergedProject.ParsleyFilters, 1)

	projectRef.ParsleyFilters = nil
	assert.NoError(t, projectRef.Upsert())
	mergedProject, err = FindMergedProjectRef("ident", "ident", true)
	assert.NoError(t, err)
	assert.Len(t, mergedProject.ParsleyFilters, 1)
}

func TestFindMergedEnabledProjectRefsByIds(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ParserProjectCollection, ProjectConfigCollection))

	projectConfig := &ProjectConfig{
		Id: "ident",
		ProjectConfigFields: ProjectConfigFields{
			TaskAnnotationSettings: &evergreen.AnnotationsSettings{
				FileTicketWebhook: evergreen.WebHook{
					Endpoint: "random2",
				},
			},
		},
	}
	assert.NoError(t, projectConfig.Insert())

	projectConfig = &ProjectConfig{
		Id: "ident2",
		ProjectConfigFields: ProjectConfigFields{
			TaskAnnotationSettings: &evergreen.AnnotationsSettings{
				FileTicketWebhook: evergreen.WebHook{
					Endpoint: "random2",
				},
			},
		},
	}
	assert.NoError(t, projectConfig.Insert())
	projectRef := &ProjectRef{
		Owner:                 "mongodb",
		RepoRefId:             "mongodb_mci",
		BatchTime:             10,
		Id:                    "ident",
		Admins:                []string{"john.smith", "john.doe"},
		Enabled:               false,
		PatchingDisabled:      utility.FalsePtr(),
		RepotrackerDisabled:   utility.TruePtr(),
		DeactivatePrevious:    utility.TruePtr(),
		VersionControlEnabled: utility.TruePtr(),
		PRTestingEnabled:      nil,
		GitTagVersionsEnabled: nil,
		GitTagAuthorizedTeams: []string{},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{ChildProject: "a different branch"},
		},
		CommitQueue:       CommitQueueParams{Enabled: nil, Message: "using repo commit queue"},
		WorkstationConfig: WorkstationConfig{GitClone: utility.TruePtr()},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.FalsePtr()},
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "project-filter",
				CaseSensitive: true,
				ExactMatch:    false,
			},
		},
	}
	assert.NoError(t, projectRef.Insert())

	projectRef = &ProjectRef{
		Owner:                 "mongodb",
		RepoRefId:             "mongodb_mci",
		BatchTime:             10,
		Id:                    "ident_enabled",
		Admins:                []string{"john.smith", "john.doe"},
		Enabled:               true,
		PatchingDisabled:      utility.FalsePtr(),
		RepotrackerDisabled:   utility.TruePtr(),
		DeactivatePrevious:    utility.TruePtr(),
		VersionControlEnabled: utility.TruePtr(),
		PRTestingEnabled:      nil,
		GitTagVersionsEnabled: nil,
		GitTagAuthorizedTeams: []string{},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{ChildProject: "a different branch"},
		},
		CommitQueue:       CommitQueueParams{Enabled: nil, Message: "using repo commit queue"},
		WorkstationConfig: WorkstationConfig{GitClone: utility.TruePtr()},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.FalsePtr()},
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "project-filter",
				CaseSensitive: true,
				ExactMatch:    false,
			},
		},
	}
	assert.NoError(t, projectRef.Insert())

	repoRef := &RepoRef{ProjectRef{
		Id:                    "mongodb_mci",
		Repo:                  "mci",
		Branch:                "main",
		SpawnHostScriptPath:   "my-path",
		Admins:                []string{"john.liu"},
		PatchingDisabled:      nil,
		GitTagVersionsEnabled: utility.FalsePtr(),
		PRTestingEnabled:      utility.TruePtr(),
		GitTagAuthorizedTeams: []string{"my team"},
		GitTagAuthorizedUsers: []string{"my user"},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{Alias: "global patch trigger"},
		},
		TaskSync:          TaskSyncOptions{ConfigEnabled: utility.TruePtr(), PatchEnabled: utility.TruePtr()},
		CommitQueue:       CommitQueueParams{Enabled: utility.TruePtr()},
		WorkstationConfig: WorkstationConfig{SetupCommands: []WorkstationSetupCommand{{Command: "my-command"}}},
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "repo-filter",
				CaseSensitive: false,
				ExactMatch:    true,
			},
		},
	}}
	assert.NoError(t, repoRef.Upsert())

	mergedProjects, err := FindMergedEnabledProjectRefsByIds("ident", "ident_enabled")
	assert.NoError(t, err)
	require.NotNil(t, mergedProjects)
	assert.Len(t, mergedProjects, 1)
	assert.Equal(t, "ident_enabled", mergedProjects[0].Id)
}
func TestGetNumberOfEnabledProjects(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	enabled1 := &ProjectRef{
		Id:      "enabled1",
		Owner:   "10gen",
		Repo:    "repo",
		Enabled: true,
	}
	assert.NoError(t, enabled1.Insert())
	enabled2 := &ProjectRef{
		Id:      "enabled2",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: true,
	}
	assert.NoError(t, enabled2.Insert())
	disabled1 := &ProjectRef{
		Id:      "disabled1",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: false,
	}
	assert.NoError(t, disabled1.Insert())
	disabled2 := &ProjectRef{
		Id:      "disabled2",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: false,
	}
	assert.NoError(t, disabled2.Insert())

	enabledProjects, err := GetNumberOfEnabledProjects()
	assert.NoError(t, err)
	assert.Equal(t, 2, enabledProjects)
	enabledProjectsOwnerRepo, err := GetNumberOfEnabledProjectsForOwnerRepo(enabled2.Owner, enabled2.Repo)
	assert.NoError(t, err)
	assert.Equal(t, 1, enabledProjectsOwnerRepo)
}

func TestValidateEnabledProjectsLimit(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))
	enabled1 := &ProjectRef{
		Id:      "enabled1",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: true,
	}
	assert.NoError(t, enabled1.Insert())
	enabled2 := &ProjectRef{
		Id:      "enabled2",
		Owner:   "owner_exception",
		Repo:    "repo_exception",
		Enabled: true,
	}
	assert.NoError(t, enabled2.Insert())
	disabled1 := &ProjectRef{
		Id:      "disabled1",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: false,
	}
	assert.NoError(t, disabled1.Insert())
	enabledByRepo := &ProjectRef{
		Id:        "enabledByRepo",
		Owner:     "enable_mongodb",
		Repo:      "enable_mci",
		RepoRefId: "enable_repo",
	}
	assert.NoError(t, enabledByRepo.Insert())
	enableRef := &RepoRef{ProjectRef{
		Id:      "enable_repo",
		Owner:   "enable_mongodb",
		Repo:    "enable_mci",
		Enabled: true,
	}}
	assert.NoError(t, enableRef.Upsert())
	disabledByRepo := &ProjectRef{
		Id:        "disabledByRepo",
		Owner:     "disable_mongodb",
		Repo:      "disable_mci",
		RepoRefId: "disable_repo",
	}
	assert.NoError(t, disabledByRepo.Insert())
	disableRepo := &RepoRef{ProjectRef{
		Id:      "disable_repo",
		Owner:   "disable_mongodb",
		Repo:    "disable_mci",
		Enabled: true,
	}}
	assert.NoError(t, disableRepo.Upsert())

	var settings evergreen.Settings
	settings.ProjectCreation.TotalProjectLimit = 4
	settings.ProjectCreation.RepoProjectLimit = 1
	settings.ProjectCreation.RepoExceptions = []evergreen.OwnerRepo{
		{
			Owner: "owner_exception",
			Repo:  "repo_exception",
		},
	}

	// Should error when trying to enable an existing project past limits.
	disabled1.Enabled = true
	original, err := FindMergedProjectRef(disabled1.Id, "", false)
	assert.NoError(t, err)
	statusCode, err := ValidateEnabledProjectsLimit(disabled1.Id, &settings, original, disabled1)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)

	// Should not error if owner/repo is part of exception.
	exception := &ProjectRef{
		Id:      "exception",
		Owner:   "owner_exception",
		Repo:    "repo_exception",
		Enabled: true,
	}
	original, err = FindMergedProjectRef(exception.Id, "", false)
	assert.NoError(t, err)
	_, err = ValidateEnabledProjectsLimit(enabled1.Id, &settings, original, exception)
	assert.NoError(t, err)

	// Should error if owner/repo is not part of exception.
	notException := &ProjectRef{
		Id:      "not_exception",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: true,
	}
	original, err = FindMergedProjectRef(notException.Id, "", false)
	assert.NoError(t, err)
	statusCode, err = ValidateEnabledProjectsLimit(notException.Id, &settings, original, notException)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)

	// Should not error if a repo defaulted project is enabled.
	disableRepo.Enabled = true
	assert.NoError(t, disableRepo.Upsert())
	mergedRef, err := GetProjectRefMergedWithRepo(*disabledByRepo)
	assert.NoError(t, err)
	original, err = FindMergedProjectRef(disabledByRepo.Id, "", false)
	assert.NoError(t, err)
	_, err = ValidateEnabledProjectsLimit(disabledByRepo.Id, &settings, original, mergedRef)
	assert.NoError(t, err)

	// Should error on enabled if you try to change owner/repo past limit.
	enabled2.Owner = "mongodb"
	enabled2.Repo = "mci"
	original, err = FindMergedProjectRef(enabled2.Id, "", false)
	assert.NoError(t, err)
	statusCode, err = ValidateEnabledProjectsLimit(enabled2.Id, &settings, original, enabled2)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)

	// Total project limit cannot be exceeded. Even with the exception.
	settings.ProjectCreation.TotalProjectLimit = 2
	original, err = FindMergedProjectRef(exception.Id, "", false)
	assert.NoError(t, err)
	statusCode, err = ValidateEnabledProjectsLimit(exception.Id, &settings, original, exception)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
}

func TestGetBatchTimeDoesNotExceedMaxBatchTime(t *testing.T) {
	assert := assert.New(t)

	projectRef := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "main",
		Enabled:   true,
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
	bvt2 := &BuildVariantTaskUnit{
		Name:    "notMyTask",
		Variant: "bv1",
		Disable: utility.TruePtr(),
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
				ActivationStatus: ActivationStatus{Activated: false, ActivateAt: prevTime.Add(-1 * time.Hour)},
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

	currentTime := time.Now()
	activationTime, err := projectRef.GetActivationTimeForTask(bvt, currentTime, time.Now())
	assert.NoError(t, err)
	assert.True(t, activationTime.Equal(prevTime.Add(time.Hour)))

	// Activation time should be the zero time, because this variant is disabled.
	activationTime, err = projectRef.GetActivationTimeForTask(bvt2, currentTime, time.Now())
	assert.NoError(t, err)
	assert.True(t, utility.IsZeroTime(activationTime))
}

func TestGetActivationTimeWithCron(t *testing.T) {
	prevTime := time.Date(2020, time.June, 9, 0, 0, 0, 0, time.UTC) // Tuesday
	for name, test := range map[string]func(t *testing.T){
		"Empty": func(t *testing.T) {
			_, err := GetNextCronTime(prevTime, "")
			assert.Error(t, err)
		},
		"InvalidBatchSyntax": func(t *testing.T) {
			batchStr := "* * *"
			_, err := GetNextCronTime(prevTime, batchStr)
			assert.Error(t, err)
		},
		"EveryHourEveryDay": func(t *testing.T) {
			batchStr := "0 * * * *"
			res, err := GetNextCronTime(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour), res)
		},
		"SpecifyDOW": func(t *testing.T) {
			batchStr := "0 0 ? * MON,WED,FRI"
			res, err := GetNextCronTime(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24), res) // i.e. Wednesday

			newRes, err := GetNextCronTime(res, batchStr) // i.e. Friday
			assert.NoError(t, err)
			assert.Equal(t, res.Add(time.Hour*48), newRes)
		},
		"1and15thOfTheMonth": func(t *testing.T) {
			batchStr := "0 0 1,15 *"
			res, err := GetNextCronTime(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24*6), res)
		},
		"Descriptor": func(t *testing.T) {
			batchStr := "@daily"
			res, err := GetNextCronTime(prevTime, batchStr)
			assert.NoError(t, err)
			assert.Equal(t, prevTime.Add(time.Hour*24), res)
		},
		"Interval": func(t *testing.T) {
			batchStr := "@every 2h"
			_, err := GetNextCronTime(prevTime, batchStr)
			assert.Error(t, err)
		},
	} {
		t.Run(name, test)
	}

	pRef := ProjectRef{
		Id:         "project",
		Identifier: "project",
	}
	versionCreatedAt, err := time.Parse(time.RFC3339, "2024-07-15T23:59:00Z")
	require.NoError(t, err)
	v := Version{
		Id:         "version",
		CreateTime: versionCreatedAt,
	}
	const cronExpr = "0 */4 * * *" // Every 4 hours
	bvtu := BuildVariantTaskUnit{
		Name:          "task_name",
		Variant:       "bv_name",
		CronBatchTime: cronExpr,
	}
	bv := BuildVariant{
		Name:          "bv_name",
		CronBatchTime: cronExpr,
	}

	for activationType, getActivationTime := range map[string]func(versionCreatedAt time.Time, now time.Time) (time.Time, error){
		"Task": func(versionCreatedAt time.Time, now time.Time) (time.Time, error) {
			return pRef.GetActivationTimeForTask(&bvtu, versionCreatedAt, now)
		},
		"Variant": func(versionCreatedAt time.Time, now time.Time) (time.Time, error) {
			return pRef.GetActivationTimeForVariant(&bv, versionCreatedAt, now)
		},
	} {
		t.Run(activationType, func(t *testing.T) {
			for tName, tCase := range map[string]func(t *testing.T, pRef *ProjectRef, v *Version, bvtu *BuildVariantTaskUnit){
				"SchedulesPastCronWithRecentlyElapsedCron": func(t *testing.T, pRef *ProjectRef, v *Version, bvtu *BuildVariantTaskUnit) {
					now := v.CreateTime.Add(2 * time.Minute)

					activateAt, err := getActivationTime(v.CreateTime, now)
					require.NoError(t, err)

					assert.True(t, activateAt.Before(now), "cron should be scheduled in the past")
				},
				"SchedulesFutureCronWithRecentlyElapsedCronButConflictingRecentCommitVersion": func(t *testing.T, pRef *ProjectRef, v *Version, bvtu *BuildVariantTaskUnit) {
					now := v.CreateTime.Add(2 * time.Minute)
					conflictingVersionWithCron := Version{
						Id:         "conflicting_version_with_cron",
						Identifier: pRef.Id,
						CreateTime: v.CreateTime.Add(-time.Hour),
						Requester:  evergreen.RepotrackerVersionRequester,
						BuildVariants: []VersionBuildStatus{
							{
								BuildVariant: "bv_name",
								ActivationStatus: ActivationStatus{
									ActivateAt: now,
								},
								BatchTimeTasks: []BatchTimeTaskStatus{
									{
										TaskName: "task_name",
										ActivationStatus: ActivationStatus{
											ActivateAt: now,
										},
									},
								},
							},
						},
					}
					require.NoError(t, conflictingVersionWithCron.Insert())

					activateAt, err := getActivationTime(v.CreateTime, now)
					require.NoError(t, err)
					assert.True(t, activateAt.After(now), "cron should be scheduled in the future due to conflicting recent commit version with recent activation time")
				},
				"SchedulesPastCronWithNonconflictingRecentCommitVersion": func(t *testing.T, pRef *ProjectRef, v *Version, bvtu *BuildVariantTaskUnit) {
					now := v.CreateTime.Add(2 * time.Minute)

					recentVersionCreatedAt := v.CreateTime.Add(-6 * time.Hour)
					recentVersionWithCron := Version{
						Id:         "conflicting_version_with_cron",
						Identifier: pRef.Id,
						CreateTime: recentVersionCreatedAt,
						Requester:  evergreen.AdHocRequester,
						BuildVariants: []VersionBuildStatus{
							{
								BuildVariant: "bv_name",
								ActivationStatus: ActivationStatus{
									ActivateAt: recentVersionCreatedAt,
								},
								BatchTimeTasks: []BatchTimeTaskStatus{
									{
										TaskName: "task_name",
										ActivationStatus: ActivationStatus{
											ActivateAt: recentVersionCreatedAt,
										},
									},
								},
							},
						},
					}
					require.NoError(t, recentVersionWithCron.Insert())

					activateAt, err := getActivationTime(v.CreateTime, now)
					require.NoError(t, err)

					assert.True(t, activateAt.Before(now), "cron should be scheduled in the past because the most recent commit version's activation time does not conflict")
				},
				"SchedulesPastCronWithRecentCommitVersionWithZeroActivationTime": func(t *testing.T, pRef *ProjectRef, v *Version, bvtu *BuildVariantTaskUnit) {
					now := v.CreateTime.Add(2 * time.Minute)

					zeroActivationTime := time.Time{}
					recentVersionWithCron := Version{
						Id:         "conflicting_version_with_cron",
						Identifier: pRef.Id,
						CreateTime: zeroActivationTime,
						Requester:  evergreen.AdHocRequester,
						BuildVariants: []VersionBuildStatus{
							{
								BuildVariant: "bv_name",
								ActivationStatus: ActivationStatus{
									ActivateAt: zeroActivationTime,
								},
								BatchTimeTasks: []BatchTimeTaskStatus{
									{
										TaskName: "task_name",
										ActivationStatus: ActivationStatus{
											ActivateAt: zeroActivationTime,
										},
									},
								},
							},
						},
					}
					require.NoError(t, recentVersionWithCron.Insert())

					activateAt, err := getActivationTime(v.CreateTime, now)
					require.NoError(t, err)

					assert.True(t, activateAt.Before(now), "cron should be scheduled in the past because the most recent commit version's activation time is zero")
				},
				"SchedulesPastCronWithNonconflictingPeriodicBuild": func(t *testing.T, pRef *ProjectRef, v *Version, bvtu *BuildVariantTaskUnit) {
					now := v.CreateTime.Add(2 * time.Minute)
					conflictingVersionWithCron := Version{
						Id:         "conflicting_version_with_cron",
						Identifier: pRef.Id,
						CreateTime: v.CreateTime.Add(-time.Hour),
						Requester:  evergreen.AdHocRequester,
						BuildVariants: []VersionBuildStatus{
							{
								BuildVariant: "bv_name",
								ActivationStatus: ActivationStatus{
									ActivateAt: now,
								},
								BatchTimeTasks: []BatchTimeTaskStatus{
									{
										TaskName: "task_name",
										ActivationStatus: ActivationStatus{
											ActivateAt: now,
										},
									},
								},
							},
						},
					}
					require.NoError(t, conflictingVersionWithCron.Insert())

					activateAt, err := getActivationTime(v.CreateTime, now)
					require.NoError(t, err)

					assert.True(t, activateAt.Before(now), "cron should be scheduled in the past since the most recent version is a periodic build")
				},
				"SchedulesFutureCronForLongElapsedCron": func(t *testing.T, pRef *ProjectRef, v *Version, bvtu *BuildVariantTaskUnit) {
					now := v.CreateTime.Add(time.Hour)

					activateAt, err := getActivationTime(v.CreateTime, now)
					require.NoError(t, err)

					assert.True(t, activateAt.After(now), "cron should be scheduled in the future because it has been a long time since the version was created")
				},
			} {
				t.Run(tName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(VersionCollection))

					tCase(t, &pRef, &v, &bvtu)
				})
			}
		})
	}
}

func TestAttachToNewRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection,
		evergreen.RoleCollection, user.Collection, evergreen.ConfigCollection, githubapp.GitHubAppCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

	settings := evergreen.Settings{
		GithubOrgs: []string{"newOwner", "evergreen-ci"},
		AuthConfig: evergreen.AuthConfig{
			Github: &evergreen.GithubAuthConfig{
				AppId: 1234,
			},
		},
		Expansions: map[string]string{
			"github_app_key": "test",
		},
	}
	assert.NoError(t, settings.Set(ctx))
	pRef := ProjectRef{
		Id:        "myProject",
		Owner:     "evergreen-ci",
		Repo:      "evergreen",
		Branch:    "main",
		Admins:    []string{"me"},
		RepoRefId: "myRepo",
		Enabled:   true,
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		PRTestingEnabled: utility.TruePtr(),
		TracksPushEvents: utility.TruePtr(),
	}
	assert.NoError(t, pRef.Insert())
	repoRef := RepoRef{ProjectRef{
		Id: "myRepo",
	}}
	assert.NoError(t, repoRef.Upsert())
	u := &user.DBUser{Id: "me",
		SystemRoles: []string{GetViewRepoRole("myRepo")},
	}
	assert.NoError(t, u.Insert())
	installation := githubapp.GitHubAppInstallation{
		Owner:          pRef.Owner,
		Repo:           pRef.Repo,
		AppID:          1234,
		InstallationID: 5678,
	}
	assert.NoError(t, installation.Upsert(ctx))

	// Can't attach to repo with an invalid owner
	pRef.Owner = "invalid"
	assert.Error(t, pRef.AttachToNewRepo(u))

	pRef.Owner = "newOwner"
	pRef.Repo = "newRepo"
	newInstallation := githubapp.GitHubAppInstallation{
		Owner:          pRef.Owner,
		Repo:           pRef.Repo,
		AppID:          1234,
		InstallationID: 5678,
	}
	assert.NoError(t, newInstallation.Upsert(ctx))
	assert.NoError(t, pRef.AttachToNewRepo(u))

	pRefFromDB, err := FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotEqual(t, pRefFromDB.RepoRefId, "myRepo")
	assert.Equal(t, pRefFromDB.Owner, "newOwner")
	assert.Equal(t, pRefFromDB.Repo, "newRepo")
	assert.Nil(t, pRefFromDB.TracksPushEvents)

	newRepoRef, err := FindOneRepoRef(pRef.RepoRefId)
	assert.NoError(t, err)
	assert.NotNil(t, newRepoRef)

	assert.True(t, newRepoRef.DoesTrackPushEvents())

	mergedRef, err := FindMergedProjectRef(pRef.Id, "", false)
	assert.NoError(t, err)
	assert.True(t, mergedRef.DoesTrackPushEvents())

	userFromDB, err := user.FindOneById("me")
	assert.NoError(t, err)
	assert.Len(t, userFromDB.SystemRoles, 2)
	assert.Contains(t, userFromDB.SystemRoles, GetRepoAdminRole(pRefFromDB.RepoRefId))
	assert.Contains(t, userFromDB.SystemRoles, GetViewRepoRole(pRefFromDB.RepoRefId))

	// Attaching a different project to this repo will result in Github conflicts being unset.
	pRef = ProjectRef{
		Id:        "mySecondProject",
		Owner:     "evergreen-ci",
		Repo:      "evergreen",
		Branch:    "main",
		Admins:    []string{"me"},
		RepoRefId: "myRepo",
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		GithubChecksEnabled: utility.TruePtr(),
		Enabled:             true,
	}
	assert.NoError(t, pRef.Insert())
	pRef.Owner = "newOwner"
	pRef.Repo = "newRepo"
	assert.NoError(t, pRef.AttachToNewRepo(u))
	assert.True(t, pRef.UseRepoSettings())
	assert.NotEmpty(t, pRef.RepoRefId)

	pRefFromDB, err = FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.True(t, pRefFromDB.UseRepoSettings())
	assert.NotEmpty(t, pRefFromDB.RepoRefId)
	// Commit queue and PR testing should be set to false, since they would introduce project conflicts.
	assert.False(t, pRefFromDB.CommitQueue.IsEnabled())
	assert.False(t, pRefFromDB.IsPRTestingEnabled())
	assert.True(t, pRefFromDB.IsGithubChecksEnabled())

}

func checkRepoAttachmentEventLog(t *testing.T, project ProjectRef, attachmentType string) {
	events, err := MostRecentProjectEvents(project.Id, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, project.Id, events[0].ResourceId)
	assert.Equal(t, event.EventResourceTypeProject, events[0].ResourceType)
	assert.Equal(t, attachmentType, events[0].EventType)
}

func TestAttachToRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection,
		evergreen.RoleCollection, user.Collection, event.EventCollection, evergreen.ConfigCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	settings := evergreen.Settings{
		GithubOrgs: []string{"newOwner", "evergreen-ci"},
	}
	assert.NoError(t, settings.Set(ctx))
	pRef := ProjectRef{
		Id:     "myProject",
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Branch: "main",
		Admins: []string{"me"},
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		GithubChecksEnabled: utility.TruePtr(),
		TracksPushEvents:    utility.TruePtr(),
		Enabled:             true,
	}
	assert.NoError(t, pRef.Insert())

	installation := githubapp.GitHubAppInstallation{
		Owner:          pRef.Owner,
		Repo:           pRef.Repo,
		AppID:          1234,
		InstallationID: 5678,
	}
	assert.NoError(t, installation.Upsert(ctx))

	u := &user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert())
	// No repo exists, but one should be created.
	assert.NoError(t, pRef.AttachToRepo(ctx, u))
	assert.True(t, pRef.UseRepoSettings())
	assert.NotEmpty(t, pRef.RepoRefId)
	checkRepoAttachmentEventLog(t, pRef, event.EventTypeProjectAttachedToRepo)

	pRefFromDB, err := FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.True(t, pRefFromDB.UseRepoSettings())
	assert.NotEmpty(t, pRefFromDB.RepoRefId)
	assert.True(t, pRefFromDB.Enabled)
	assert.True(t, pRefFromDB.CommitQueue.IsEnabled())
	assert.True(t, pRefFromDB.IsGithubChecksEnabled())
	assert.Nil(t, pRefFromDB.TracksPushEvents)

	repoRef, err := FindOneRepoRef(pRef.RepoRefId)
	assert.NoError(t, err)
	require.NotNil(t, repoRef)
	assert.True(t, repoRef.DoesTrackPushEvents())

	u, err = user.FindOneById("me")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Contains(t, u.Roles(), GetViewRepoRole(pRefFromDB.RepoRefId))
	assert.Contains(t, u.Roles(), GetRepoAdminRole(pRefFromDB.RepoRefId))

	// Try attaching a new project ref, now that a repo does exist.
	pRef = ProjectRef{
		Id:     "mySecondProject",
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Branch: "main",
		Admins: []string{"me"},
		CommitQueue: CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		PRTestingEnabled: utility.TruePtr(),
		Enabled:          true,
	}
	assert.NoError(t, pRef.Insert())
	assert.NoError(t, pRef.AttachToRepo(ctx, u))
	assert.True(t, pRef.UseRepoSettings())
	assert.NotEmpty(t, pRef.RepoRefId)
	checkRepoAttachmentEventLog(t, pRef, event.EventTypeProjectAttachedToRepo)

	pRefFromDB, err = FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.True(t, pRefFromDB.UseRepoSettings())
	assert.NotEmpty(t, pRefFromDB.RepoRefId)
	// Commit queue and github checks should be set to false, since they would introduce project conflicts.
	assert.False(t, pRefFromDB.CommitQueue.IsEnabled())
	assert.False(t, pRefFromDB.IsGithubChecksEnabled())
	assert.True(t, pRefFromDB.IsPRTestingEnabled())

	// Try attaching with a disallowed owner.
	pRef = ProjectRef{
		Id:     "myBadProject",
		Owner:  "nonexistent",
		Repo:   "evergreen",
		Branch: "main",
	}
	assert.NoError(t, pRef.Insert())
	assert.Error(t, pRef.AttachToRepo(ctx, u))
}

func TestDetachFromRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for name, test := range map[string]func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser){
		"project ref is updated correctly": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			pRefFromDB, err := FindBranchProjectRef(pRef.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.False(t, pRefFromDB.UseRepoSettings())
			assert.Empty(t, pRefFromDB.RepoRefId)
			assert.NotNil(t, pRefFromDB.PRTestingEnabled)
			assert.False(t, pRefFromDB.IsPRTestingEnabled())
			assert.NotNil(t, pRefFromDB.GitTagVersionsEnabled)
			assert.True(t, pRefFromDB.IsGitTagVersionsEnabled())
			assert.True(t, pRefFromDB.IsGithubChecksEnabled())
			assert.Equal(t, pRefFromDB.GithubTriggerAliases, []string{"my_trigger"})
			assert.True(t, pRefFromDB.DoesTrackPushEvents())

			dbUser, err = user.FindOneById("me")
			assert.NoError(t, err)
			assert.NotNil(t, dbUser)
			assert.NotContains(t, dbUser.Roles(), GetViewRepoRole(pRefFromDB.RepoRefId))
		},
		"project variables are updated": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			vars, err := FindOneProjectVars(pRef.Id)
			assert.NoError(t, err)
			assert.NotNil(t, vars)
			assert.Equal(t, vars.Vars["project"], "only")
			assert.Equal(t, vars.Vars["in"], "both")    // not modified
			assert.Equal(t, vars.Vars["repo"], "only!") // added from repo
			assert.False(t, vars.PrivateVars["project"])
			assert.True(t, vars.PrivateVars["in"])
			assert.True(t, vars.PrivateVars["repo"]) // added from repo
		},
		"patch aliases": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			// no patch aliases are copied if the project has a patch alias
			projectAlias := ProjectAlias{Alias: "myProjectAlias", ProjectID: pRef.Id}
			assert.NoError(t, projectAlias.Upsert())

			repoAlias := ProjectAlias{Alias: "myRepoAlias", ProjectID: pRef.RepoRefId}
			assert.NoError(t, repoAlias.Upsert())

			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			aliases, err := FindAliasesForProjectFromDb(pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			assert.Equal(t, aliases[0].Alias, projectAlias.Alias)

			// reattach to repo to test without project patch aliases
			assert.NoError(t, pRef.AttachToRepo(ctx, dbUser))
			assert.NotEmpty(t, pRef.RepoRefId)
			assert.True(t, pRef.UseRepoSettings())
			assert.NoError(t, RemoveProjectAlias(projectAlias.ID.Hex()))

			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			aliases, err = FindAliasesForProjectFromDb(pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			assert.Equal(t, aliases[0].Alias, repoAlias.Alias)

		},
		"internal aliases": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			projectAliases := []ProjectAlias{
				{Alias: evergreen.GitTagAlias, Variant: "projectVariant"},
				{Alias: evergreen.CommitQueueAlias},
			}
			assert.NoError(t, UpsertAliasesForProject(projectAliases, pRef.Id))
			repoAliases := []ProjectAlias{
				{Alias: evergreen.GitTagAlias, Variant: "repoVariant"},
				{Alias: evergreen.GithubPRAlias},
			}
			assert.NoError(t, UpsertAliasesForProject(repoAliases, pRef.RepoRefId))

			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			aliases, err := FindAliasesForProjectFromDb(pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 3)
			gitTagCount := 0
			prCount := 0
			cqCount := 0
			for _, a := range aliases {
				if a.Alias == evergreen.GitTagAlias {
					gitTagCount += 1
					assert.Equal(t, a.Variant, projectAliases[0].Variant) // wasn't overwritten by repo
				}
				if a.Alias == evergreen.GithubPRAlias {
					prCount += 1
				}
				if a.Alias == evergreen.CommitQueueAlias {
					cqCount += 1
				}
			}
			assert.Equal(t, gitTagCount, 1)
			assert.Equal(t, prCount, 1)
			assert.Equal(t, cqCount, 1)
		},
		"subscriptions": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			projectSubscription := event.Subscription{
				Owner:        pRef.Id,
				OwnerType:    event.OwnerTypeProject,
				ResourceType: event.ResourceTypeTask,
				Trigger:      event.TriggerOutcome,
				Selectors: []event.Selector{
					{Type: "id", Data: "1234"},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, projectSubscription.Upsert())
			repoSubscription := event.Subscription{
				Owner:        pRef.RepoRefId,
				OwnerType:    event.OwnerTypeProject,
				ResourceType: event.ResourceTypeTask,
				Trigger:      event.TriggerFailure,
				Selectors: []event.Selector{
					{Type: "id", Data: "1234"},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, repoSubscription.Upsert())
			assert.NoError(t, pRef.DetachFromRepo(dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)

			subs, err := event.FindSubscriptionsByOwner(pRef.Id, event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, subs, 1)
			assert.Equal(t, subs[0].Owner, pRef.Id)
			assert.Equal(t, subs[0].Trigger, event.TriggerOutcome)

			// reattach to repo to test without subscription
			assert.NoError(t, pRef.AttachToRepo(ctx, dbUser))
			assert.NoError(t, event.RemoveSubscription(projectSubscription.ID))
			assert.NoError(t, pRef.DetachFromRepo(dbUser))

			subs, err = event.FindSubscriptionsByOwner(pRef.Id, event.OwnerTypeProject)
			assert.NoError(t, err)
			assert.Len(t, subs, 1)
			assert.Equal(t, subs[0].Owner, pRef.Id)
			assert.Equal(t, subs[0].Trigger, event.TriggerFailure)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection,
				evergreen.RoleCollection, user.Collection, event.SubscriptionsCollection, event.EventCollection, ProjectAliasCollection))
			require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

			pRef := &ProjectRef{
				Id:        "myProject",
				Owner:     "evergreen-ci",
				Repo:      "evergreen",
				Admins:    []string{"me"},
				RepoRefId: "myRepo",

				PeriodicBuilds:        []PeriodicBuildDefinition{}, // also shouldn't be overwritten
				PRTestingEnabled:      utility.FalsePtr(),          // neither of these should be changed when overwriting
				GitTagVersionsEnabled: utility.TruePtr(),
				GithubChecksEnabled:   nil, // for now this is defaulting to repo
				//GithubTriggerAliases:  nil,
			}
			assert.NoError(t, pRef.Insert())

			repoRef := RepoRef{ProjectRef{
				Id:                    pRef.RepoRefId,
				Owner:                 pRef.Owner,
				Repo:                  pRef.Repo,
				TracksPushEvents:      utility.TruePtr(),
				PRTestingEnabled:      utility.TruePtr(),
				GitTagVersionsEnabled: utility.FalsePtr(),
				GithubChecksEnabled:   utility.TruePtr(),
				GithubTriggerAliases:  []string{"my_trigger"},
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "my_build"},
				},
			}}
			assert.NoError(t, repoRef.Upsert())

			pVars := &ProjectVars{
				Id: pRef.Id,
				Vars: map[string]string{
					"project": "only",
					"in":      "both",
				},
				PrivateVars: map[string]bool{
					"in": true,
				},
			}
			_, err := pVars.Upsert()
			assert.NoError(t, err)

			repoVars := &ProjectVars{
				Id: repoRef.Id,
				Vars: map[string]string{
					"in":   "also the repo",
					"repo": "only!",
				},
				PrivateVars: map[string]bool{
					"repo": true,
				},
			}
			_, err = repoVars.Upsert()
			assert.NoError(t, err)

			u := &user.DBUser{
				Id:          "me",
				SystemRoles: []string{GetViewRepoRole("myRepo")},
			}
			assert.NoError(t, u.Insert())
			test(t, pRef, u)
		})
	}
}

func TestDefaultRepoBySection(t *testing.T) {
	for name, test := range map[string]func(t *testing.T, id string){
		ProjectPageGeneralSection: func(t *testing.T, id string) {
			repoRef := RepoRef{
				ProjectRef: ProjectRef{
					Id:      "repo_ref_id",
					Owner:   "mongodb",
					Repo:    "mci",
					Branch:  "main",
					Enabled: false,
				},
			}
			assert.NoError(t, repoRef.Upsert())
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageGeneralSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.NotEqual(t, pRefFromDb.Identifier, "")
			assert.Equal(t, pRefFromDb.BatchTime, 0)
			assert.Nil(t, pRefFromDb.RepotrackerDisabled)
			assert.Nil(t, pRefFromDb.DeactivatePrevious)
			assert.Empty(t, pRefFromDb.RemotePath)
			assert.Nil(t, pRefFromDb.TaskSync.ConfigEnabled)
		},
		ProjectPageAccessSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageAccessSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.Restricted)
			assert.Nil(t, pRefFromDb.Admins)
		},
		ProjectPageVariablesSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageVariablesSection, "me"))

			varsFromDb, err := FindOneProjectVars(id)
			assert.NoError(t, err)
			assert.NotNil(t, varsFromDb)
			assert.Nil(t, varsFromDb.Vars)
			assert.Nil(t, varsFromDb.PrivateVars)
			assert.NotEmpty(t, varsFromDb.Id)
		},
		ProjectPageGithubAndCQSection: func(t *testing.T, id string) {
			aliases, err := FindAliasesForProjectFromDb(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 5)
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageGithubAndCQSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PRTestingEnabled)
			assert.Nil(t, pRefFromDb.GithubChecksEnabled)
			assert.Nil(t, pRefFromDb.GitTagAuthorizedUsers)
			aliases, err = FindAliasesForProjectFromDb(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			// assert that only patch aliases are left
			for _, a := range aliases {
				assert.NotContains(t, evergreen.InternalAliases, a.Alias)
			}
		},
		ProjectPageNotificationsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageNotificationsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.NotifyOnBuildFailure)
		},
		ProjectPagePatchAliasSection: func(t *testing.T, id string) {
			aliases, err := FindAliasesForProjectFromDb(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 5)

			assert.NoError(t, DefaultSectionToRepo(id, ProjectPagePatchAliasSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PatchTriggerAliases)

			aliases, err = FindAliasesForProjectFromDb(id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 4)
			// assert that no patch aliases are left
			for _, a := range aliases {
				assert.Contains(t, evergreen.InternalAliases, a.Alias)
			}
		},
		ProjectPageTriggersSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageTriggersSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.Triggers)
		},
		ProjectPageWorkstationsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPageWorkstationsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.WorkstationConfig.GitClone)
			assert.Nil(t, pRefFromDb.WorkstationConfig.SetupCommands)
		},
		ProjectPagePluginSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPagePluginSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Equal(t, pRefFromDb.TaskAnnotationSettings.FileTicketWebhook.Endpoint, "")
			assert.Equal(t, pRefFromDb.BuildBaronSettings.TicketCreateProject, "")
			assert.Nil(t, pRefFromDb.PerfEnabled)
		},
		ProjectPagePeriodicBuildsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(id, ProjectPagePeriodicBuildsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PeriodicBuilds)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(ProjectRefCollection, ProjectVarsCollection, ProjectAliasCollection,
				event.SubscriptionsCollection, event.EventCollection, RepoRefCollection))

			pRef := ProjectRef{
				Id:                    "my_project",
				Identifier:            "my_identifier",
				Owner:                 "candy",
				Repo:                  "land",
				BatchTime:             10,
				RepotrackerDisabled:   utility.TruePtr(),
				DeactivatePrevious:    utility.FalsePtr(),
				RemotePath:            "path.yml",
				TaskSync:              TaskSyncOptions{ConfigEnabled: utility.TruePtr()},
				Restricted:            utility.FalsePtr(),
				Admins:                []string{"annie"},
				PRTestingEnabled:      utility.TruePtr(),
				GithubChecksEnabled:   utility.FalsePtr(),
				GitTagAuthorizedUsers: []string{"anna"},
				NotifyOnBuildFailure:  utility.FalsePtr(),
				PerfEnabled:           utility.FalsePtr(),
				RepoRefId:             "repo_ref_id",
				Triggers: []TriggerDefinition{
					{Project: "your_project"},
				},
				PatchTriggerAliases: []patch.PatchTriggerDefinition{
					{ChildProject: "your_project"},
				},
				WorkstationConfig: WorkstationConfig{
					GitClone: utility.TruePtr(),
					SetupCommands: []WorkstationSetupCommand{
						{Command: "expeliarmus"},
					},
				},
				PeriodicBuilds: []PeriodicBuildDefinition{
					{
						ID:         "so_occasional",
						ConfigFile: "build.yml",
					},
				},
				TaskAnnotationSettings: evergreen.AnnotationsSettings{
					FileTicketWebhook: evergreen.WebHook{
						Endpoint: "random1",
					},
				},
				BuildBaronSettings: evergreen.BuildBaronSettings{
					TicketCreateProject:  "BFG",
					TicketSearchProjects: []string{"BF", "BFG"},
				},
			}
			assert.NoError(t, pRef.Insert())

			pVars := ProjectVars{
				Id:          pRef.Id,
				Vars:        map[string]string{"hello": "world"},
				PrivateVars: map[string]bool{"hello": true},
			}
			assert.NoError(t, pVars.Insert())

			aliases := []ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.GithubPRAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.GitTagAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.CommitQueueAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     evergreen.GithubChecksAlias,
					Variant:   "v",
					Task:      "t",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: pRef.Id,
					Alias:     "i am a patch alias!",
					Variant:   "v",
					Task:      "t",
				},
			}
			for _, a := range aliases {
				assert.NoError(t, a.Upsert())
			}
			test(t, pRef.Id)
		})
	}
}

func TestGetGitHubProjectConflicts(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	// Two project refs that are from different repos should never conflict.
	p1 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci1",
		Branch:  "main",
		Id:      "p1",
		Enabled: true,
	}
	require.NoError(p1.Insert())
	p2 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "not-mci1",
		Branch:  "main",
		Id:      "p2",
		Enabled: true,
	}
	require.NoError(p2.Insert())
	conflicts, err := p1.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 0)
	assert.Len(conflicts.CommitQueueIdentifiers, 0)
	assert.Len(conflicts.CommitCheckIdentifiers, 0)

	// Two project refs that are from the same repo but do not have potential conflicting settings.
	p3 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci2",
		Branch:  "main",
		Id:      "p3",
		Enabled: true,
	}
	require.NoError(p3.Insert())
	p4 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci2",
		Branch:  "main",
		Id:      "p4",
		Enabled: true,
	}
	require.NoError(p4.Insert())
	conflicts, err = p3.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 0)
	assert.Len(conflicts.CommitQueueIdentifiers, 0)
	assert.Len(conflicts.CommitCheckIdentifiers, 0)

	// Three project refs that do have potential conflicting settings.
	p5 := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci3",
		Branch:           "main",
		Id:               "p5",
		Enabled:          true,
		PRTestingEnabled: utility.TruePtr(),
	}
	require.NoError(p5.Insert())
	p6 := &ProjectRef{
		Owner:       "mongodb",
		Repo:        "mci3",
		Branch:      "main",
		Id:          "p6",
		Enabled:     true,
		CommitQueue: CommitQueueParams{Enabled: utility.TruePtr()},
	}
	require.NoError(p6.Insert())
	p7 := &ProjectRef{
		Owner:               "mongodb",
		Repo:                "mci3",
		Branch:              "main",
		Id:                  "p7",
		Enabled:             true,
		GithubChecksEnabled: utility.TruePtr(),
	}
	require.NoError(p7.Insert())
	p8 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci3",
		Branch:  "main",
		Id:      "p8",
		Enabled: true,
	}
	require.NoError(p8.Insert())
	// p5 should have conflicting with commit queue and commit check.
	conflicts, err = p5.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 0)
	assert.Len(conflicts.CommitQueueIdentifiers, 1)
	assert.Len(conflicts.CommitCheckIdentifiers, 1)
	// p6 should have conflicting with pr testing and commit check.
	conflicts, err = p6.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 1)
	assert.Len(conflicts.CommitQueueIdentifiers, 0)
	assert.Len(conflicts.CommitCheckIdentifiers, 1)
	// p7 should have conflicting with pr testing and commit queue.
	conflicts, err = p7.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 1)
	assert.Len(conflicts.CommitQueueIdentifiers, 1)
	assert.Len(conflicts.CommitCheckIdentifiers, 0)
	// p8 should have conflicting with all
	conflicts, err = p8.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 1)
	assert.Len(conflicts.CommitQueueIdentifiers, 1)
	assert.Len(conflicts.CommitCheckIdentifiers, 1)

	// Two project refs in which one is the 'parent' or repo tracking project while the other is
	// a branch tracking project that has their RepoRefId set to the 'parent'. And because
	// the branch tracking project inherits the settings, it should not conflict.
	p9 := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci4",
		Branch:           "main",
		Id:               "p9",
		Enabled:          true,
		PRTestingEnabled: utility.TruePtr(),
	}
	require.NoError(p9.Insert())
	r9 := &RepoRef{
		ProjectRef: *p9,
	}
	require.NoError(r9.Upsert())
	p10 := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci4",
		Branch:    "main",
		Id:        "p10",
		Enabled:   true,
		RepoRefId: p9.Id,
	}
	require.NoError(p10.Insert())
	// p9 should not have any potential conflicts.
	conflicts, err = p9.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 0)
	assert.Len(conflicts.CommitQueueIdentifiers, 0)
	assert.Len(conflicts.CommitCheckIdentifiers, 0)
	// p10 should have a potential conflict because p9 has something enabled.
	conflicts, err = p10.GetGithubProjectConflicts()
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 1)
	assert.Len(conflicts.CommitQueueIdentifiers, 0)
	assert.Len(conflicts.CommitCheckIdentifiers, 0)
}

func TestFindProjectRefsByRepoAndBranch(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	assert.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	projectRefs, err := FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "main",
		Enabled:          false,
		BatchTime:        10,
		Id:               "iden_",
		PRTestingEnabled: utility.TruePtr(),
	}
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef.Id = "ident"
	projectRef.Enabled = true
	assert.NoError(projectRef.Insert())

	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	require.Len(projectRefs, 1)
	assert.Equal("ident", projectRefs[0].Id)

	projectRef.Id = "ident2"
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Len(projectRefs, 2)
}

func TestSetGithubAppCredentials(t *testing.T) {
	sampleAppId := int64(10)
	samplePrivateKey := []byte("private_key")
	for name, test := range map[string]func(t *testing.T, p *ProjectRef){
		"NoCredentialsWhenNoneExist": func(t *testing.T, p *ProjectRef) {
			app, err := githubapp.FindOneGithubAppAuth(p.Id)
			require.NoError(t, err)
			assert.Nil(t, app)
		},
		"CredentialsCanBeSet": func(t *testing.T, p *ProjectRef) {
			require.NoError(t, p.SetGithubAppCredentials(sampleAppId, samplePrivateKey))
			app, err := githubapp.FindOneGithubAppAuth(p.Id)
			require.NoError(t, err)
			assert.Equal(t, sampleAppId, app.AppID)
			assert.Equal(t, samplePrivateKey, app.PrivateKey)
		},
		"CredentialsCanBeRemovedByEmptyAppIDAndEmptyPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(sampleAppId, samplePrivateKey))
			app, err := githubapp.FindOneGithubAppAuth(p.Id)
			require.NoError(t, err)
			assert.Equal(t, sampleAppId, app.AppID)
			assert.Equal(t, samplePrivateKey, app.PrivateKey)

			// Remove credentials.
			require.NoError(t, p.SetGithubAppCredentials(0, []byte("")))
			app, err = githubapp.FindOneGithubAppAuth(p.Id)
			require.NoError(t, err)
			assert.Nil(t, app)
		},
		"CredentialsCanBeRemovedByEmptyAppIDAndNilPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(sampleAppId, samplePrivateKey))
			app, err := githubapp.FindOneGithubAppAuth(p.Id)
			require.NoError(t, err)
			assert.Equal(t, sampleAppId, app.AppID)
			assert.Equal(t, samplePrivateKey, app.PrivateKey)

			// Remove credentials.
			require.NoError(t, p.SetGithubAppCredentials(0, nil))
			app, err = githubapp.FindOneGithubAppAuth(p.Id)
			require.NoError(t, err)
			assert.Nil(t, app)
		},
		"CredentialsCannotBeRemovedByOnlyEmptyPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(sampleAppId, samplePrivateKey))
			appID, err := githubapp.GetGitHubAppID(p.Id)
			require.NoError(t, err)
			assert.NotNil(t, appID)

			// Remove credentials.
			require.Error(t, p.SetGithubAppCredentials(sampleAppId, []byte("")), "both app ID and private key must be provided")
		},
		"CredentialsCannotBeRemovedByOnlyNilPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(10, samplePrivateKey))
			appID, err := githubapp.GetGitHubAppID(p.Id)
			require.NoError(t, err)
			assert.NotNil(t, appID)

			// Remove credentials.
			require.Error(t, p.SetGithubAppCredentials(sampleAppId, nil), "both app ID and private key must be provided")
		},
		"CredentialsCannotBeRemovedByOnlyEmptyAppID": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(sampleAppId, samplePrivateKey))
			appID, err := githubapp.GetGitHubAppID(p.Id)
			require.NoError(t, err)
			assert.NotNil(t, appID)

			// Remove credentials.
			require.Error(t, p.SetGithubAppCredentials(0, samplePrivateKey), "both app ID and private key must be provided")
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectRefCollection, githubapp.GitHubAppAuthCollection))
			p := &ProjectRef{
				Id: "id1",
			}
			require.NoError(t, p.Insert())
			test(t, p)
		})
	}
}

func TestCreateNewRepoRef(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, user.Collection,
		evergreen.ScopeCollection, ProjectVarsCollection, ProjectAliasCollection, githubapp.GitHubAppCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	doc1 := &ProjectRef{
		Id:                   "id1",
		Owner:                "mongodb",
		Repo:                 "mongo",
		Branch:               "mci",
		Enabled:              true,
		Admins:               []string{"bob", "other bob"},
		PRTestingEnabled:     utility.TruePtr(),
		RemotePath:           "evergreen.yml",
		NotifyOnBuildFailure: utility.TruePtr(),
		CommitQueue:          CommitQueueParams{Message: "my message"},
		TaskSync:             TaskSyncOptions{PatchEnabled: utility.TruePtr()},
	}
	assert.NoError(t, doc1.Insert())
	doc2 := &ProjectRef{
		Id:                   "id2",
		Identifier:           "identifier",
		Owner:                "mongodb",
		Repo:                 "mongo",
		Branch:               "mci2",
		Enabled:              true,
		Admins:               []string{"bob", "other bob"},
		PRTestingEnabled:     utility.TruePtr(),
		RemotePath:           "evergreen.yml",
		NotifyOnBuildFailure: utility.FalsePtr(),
		GithubChecksEnabled:  utility.TruePtr(),
		CommitQueue:          CommitQueueParams{Message: "my message"},
		TaskSync:             TaskSyncOptions{PatchEnabled: utility.TruePtr(), ConfigEnabled: utility.TruePtr()},
	}
	assert.NoError(t, doc2.Insert())
	doc3 := &ProjectRef{
		Id:      "id3",
		Owner:   "mongodb",
		Repo:    "mongo",
		Branch:  "mci2",
		Enabled: false,
	}
	assert.NoError(t, doc3.Insert())

	installation := githubapp.GitHubAppInstallation{
		Owner:          "mongodb",
		Repo:           "mongo",
		AppID:          1234,
		InstallationID: 5678,
	}
	assert.NoError(t, installation.Upsert(ctx))

	projectVariables := []ProjectVars{
		{
			Id: doc1.Id,
			Vars: map[string]string{
				"hello":        "world",
				"sdc":          "buggy",
				"violets":      "nah",
				"roses":        "red",
				"ever":         "green",
				"also":         "this one",
				"this is only": "in one doc",
			},
			PrivateVars: map[string]bool{
				"sdc": true,
			},
		},
		{
			Id: doc2.Id,
			Vars: map[string]string{
				"hello":   "world",
				"violets": "blue",
				"sdc":     "buggy",
				"ever":    "green",
			},
		},
		{
			Id: doc3.Id,
			Vars: map[string]string{
				"it's me": "adele",
			},
		},
	}
	for _, vars := range projectVariables {
		assert.NoError(t, vars.Insert())
	}

	projectAliases := ProjectAliases{
		ProjectAlias{
			ProjectID: doc1.Id,
			Task:      ".*",
			Variant:   ".*",
			Alias:     evergreen.GithubPRAlias,
		},
		ProjectAlias{
			ProjectID: doc2.Id,
			Task:      ".*",
			Variant:   ".*",
			Alias:     evergreen.GithubPRAlias,
		},
		ProjectAlias{
			ProjectID: doc1.Id,
			TaskTags:  []string{"t2"},
			Variant:   ".*",
			Alias:     evergreen.GithubChecksAlias,
		},
		ProjectAlias{
			ProjectID: doc2.Id,
			TaskTags:  []string{"t1"},
			Variant:   ".*",
			Alias:     evergreen.GithubChecksAlias,
		},
		ProjectAlias{
			ProjectID:   doc1.Id,
			Task:        ".*",
			VariantTags: []string{"v1"},
			Alias:       evergreen.GitTagAlias,
		},
		ProjectAlias{
			ProjectID:   doc2.Id,
			Task:        ".*",
			VariantTags: []string{"v1"},
			Alias:       evergreen.GitTagAlias,
		},
		ProjectAlias{
			ProjectID:  doc1.Id,
			RemotePath: "random",
			Alias:      "random-alias",
		},
	}
	for _, a := range projectAliases {
		assert.NoError(t, a.Upsert())
	}
	u := user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert())
	// This will create the new repo ref
	assert.NoError(t, doc2.AddToRepoScope(&u))
	assert.NotEmpty(t, doc2.RepoRefId)

	repoRef, err := FindOneRepoRef(doc2.RepoRefId)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)

	assert.Equal(t, "mongodb", repoRef.Owner)
	assert.Equal(t, "mongo", repoRef.Repo)
	assert.Empty(t, repoRef.Branch)
	assert.True(t, repoRef.DoesTrackPushEvents())
	assert.Contains(t, repoRef.Admins, "bob")
	assert.Contains(t, repoRef.Admins, "other bob")
	assert.Contains(t, repoRef.Admins, "me")
	assert.True(t, repoRef.IsPRTestingEnabled())
	assert.Equal(t, "evergreen.yml", repoRef.RemotePath)
	assert.Equal(t, "", repoRef.Identifier)
	assert.Nil(t, repoRef.NotifyOnBuildFailure)
	assert.Nil(t, repoRef.GithubChecksEnabled)
	assert.Equal(t, "my message", repoRef.CommitQueue.Message)
	assert.False(t, repoRef.TaskSync.IsPatchEnabled())

	projectVars, err := FindOneProjectVars(repoRef.Id)
	assert.NoError(t, err)
	assert.Len(t, projectVars.Vars, 3)
	assert.Len(t, projectVars.PrivateVars, 1)
	assert.Equal(t, "world", projectVars.Vars["hello"])
	assert.Equal(t, "buggy", projectVars.Vars["sdc"])
	assert.Equal(t, "green", projectVars.Vars["ever"])
	assert.True(t, projectVars.PrivateVars["sdc"])

	projectAliases, err = FindAliasesForRepo(repoRef.Id)
	assert.NoError(t, err)
	assert.Len(t, projectAliases, 2)
	for _, a := range projectAliases {
		assert.Empty(t, a.RemotePath)
		assert.Empty(t, a.GitTag)
		assert.Empty(t, a.TaskTags)
		if a.Alias == evergreen.GithubPRAlias {
			assert.Equal(t, ".*", a.Task)
			assert.Equal(t, ".*", a.Variant)
			assert.Empty(t, a.VariantTags)
		} else {
			assert.Equal(t, evergreen.GitTagAlias, a.Alias)
			assert.Equal(t, ".*", a.Task)
			assert.Contains(t, a.VariantTags, "v1")
		}
	}

	env := testutil.NewEnvironment(ctx, t)
	// verify that both the project and repo are part of the scope
	rm := env.RoleManager()
	scope, err := rm.GetScope(context.TODO(), GetRepoAdminScope(repoRef.Id))
	assert.NoError(t, err)
	assert.NotNil(t, scope)
	assert.Contains(t, scope.Resources, repoRef.Id)
	assert.Contains(t, scope.Resources, doc2.Id)
	assert.NotContains(t, scope.Resources, doc1.Id)
}

func TestGithubPermissionGroups(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(db.CreateCollections(evergreen.ScopeCollection))

	orgGroup := []GitHubDynamicTokenPermissionGroup{
		{
			Name: "some-group",
			Permissions: github.InstallationPermissions{
				Administration:             utility.ToStringPtr("admin"),
				Actions:                    utility.ToStringPtr("read"),
				Contents:                   utility.ToStringPtr("write"),
				Checks:                     utility.ToStringPtr("write"),
				Metadata:                   utility.ToStringPtr("write"),
				OrganizationAdministration: utility.ToStringPtr("admin"),
			},
		},
		{
			Name: "other-group",
			Permissions: github.InstallationPermissions{
				Administration:             utility.ToStringPtr("write"),
				Actions:                    utility.ToStringPtr("write"),
				Checks:                     utility.ToStringPtr("read"),
				Metadata:                   utility.ToStringPtr("write"),
				OrganizationAdministration: utility.ToStringPtr("admin"),
			},
		},
		{
			Name: "no-permissions",
		},
		{
			Name: "fake-permissions",
			Permissions: github.InstallationPermissions{
				Administration: utility.ToStringPtr("not-a-permission"),
			},
		},
		{
			Name:           "all-permissions-1",
			AllPermissions: true,
		},
		{
			Name:           "all-permissions-2",
			AllPermissions: true,
		},
		{
			Name: "other-permissions",
			Permissions: github.InstallationPermissions{
				Contents: utility.ToStringPtr("read"),
			},
		},
	}
	orgRequesters := map[string]string{
		evergreen.PatchVersionRequester: "some-group",
		evergreen.GithubPRRequester:     noPermissionsGitHubTokenPermissionGroup.Name,
	}
	p := &ProjectRef{
		GitHubDynamicTokenPermissionGroups: orgGroup,
		GitHubPermissionGroupByRequester:   orgRequesters,
	}
	require.NoError(p.Insert())

	t.Run("Not found requester should return default permissions", func(t *testing.T) {
		group, found := p.GetGitHubPermissionGroup("requester")
		assert.Equal(defaultGitHubTokenPermissionGroup, group)
		assert.False(found)
	})

	t.Run("Found requester should return correct group", func(t *testing.T) {
		group, found := p.GetGitHubPermissionGroup(evergreen.PatchVersionRequester)
		assert.Equal("some-group", group.Name)
		assert.True(found)
		assert.Equal("read", utility.FromStringPtr(group.Permissions.Actions))
	})

	t.Run("Valid group passes validation", func(t *testing.T) {
		assert.NoError(p.ValidateGitHubPermissionGroups())
	})

	t.Run("Invalid name in group fails validation", func(t *testing.T) {
		p.GitHubDynamicTokenPermissionGroups = append(orgGroup,
			GitHubDynamicTokenPermissionGroup{
				Name: "",
			},
		)
		assert.ErrorContains(p.ValidateGitHubPermissionGroups(), "group name cannot be empty")
	})

	t.Run("Invalid requester in group fails validation", func(t *testing.T) {
		p.GitHubPermissionGroupByRequester = map[string]string{
			"second-requester": "some-group",
		}
		assert.ErrorContains(p.ValidateGitHubPermissionGroups(), "requester 'second-requester' is not a valid requester")
	})

	t.Run("Valid requester pointing to not found group fails validation", func(t *testing.T) {
		p.GitHubPermissionGroupByRequester = map[string]string{
			evergreen.GithubPRRequester: "second-group",
		}
		assert.ErrorContains(p.ValidateGitHubPermissionGroups(), fmt.Sprintf("group 'second-group' for requester '%s' not found", evergreen.GithubPRRequester))
	})

	t.Run("Intersection of permissions should return most restrictive", func(t *testing.T) {
		intersection, err := orgGroup[0].Intersection(orgGroup[1])
		require.NoError(err)
		assert.Equal(orgGroup[0].Name, intersection.Name)
		assert.False(intersection.AllPermissions)

		assert.Equal("write", utility.FromStringPtr(intersection.Permissions.Administration), "write and admin should restrict to write")
		assert.Equal("read", utility.FromStringPtr(intersection.Permissions.Actions), "read and write should restrict to read")
		assert.Nil(intersection.Permissions.Contents, "nil and write should restrict to nil")
		assert.Nil(intersection.Permissions.Followers, "both nil should restrict to nil")
		assert.Equal("read", utility.FromStringPtr(intersection.Permissions.Checks), "write and read should restrict to read")
		assert.Equal("write", utility.FromStringPtr(intersection.Permissions.Metadata), "both write should restrict to write")
		assert.Equal("admin", utility.FromStringPtr(intersection.Permissions.OrganizationAdministration), "both admin should restrict to admin")

		assert.Nil(intersection.Permissions.Emails, "an unspecified field should restrict to nil")
	})

	t.Run("Intersection of permissions with no permissions should return no permissions", func(t *testing.T) {
		intersection, err := orgGroup[0].Intersection(orgGroup[2])
		require.NoError(err)
		assert.Equal(orgGroup[0].Name, intersection.Name)

		assert.True(intersection.HasNoPermissions())
	})

	t.Run("Intersection of two no permissions should return no permissions", func(t *testing.T) {
		intersection, err := orgGroup[2].Intersection(orgGroup[2])
		require.NoError(err)
		assert.Equal(orgGroup[2].Name, intersection.Name)
		assert.True(intersection.HasNoPermissions())

		// Specified fields.
		assert.Nil(intersection.Permissions.Administration)
		assert.Nil(intersection.Permissions.Actions)
		assert.Nil(intersection.Permissions.Contents)
		assert.Nil(intersection.Permissions.Followers)
		assert.Nil(intersection.Permissions.Checks)
		assert.Nil(intersection.Permissions.Metadata)
		assert.Nil(intersection.Permissions.OrganizationAdministration)

		// An unspecified field.
		assert.Nil(intersection.Permissions.Emails)

		intersection, err = noPermissionsGitHubTokenPermissionGroup.Intersection(orgGroup[2])
		require.NoError(err)
		assert.Equal(noPermissionsGitHubTokenPermissionGroup.Name, intersection.Name)
		assert.True(intersection.HasNoPermissions())

		// Specified fields.
		assert.Nil(intersection.Permissions.Administration)
		assert.Nil(intersection.Permissions.Actions)
		assert.Nil(intersection.Permissions.Contents)
		assert.Nil(intersection.Permissions.Followers)
		assert.Nil(intersection.Permissions.Checks)
		assert.Nil(intersection.Permissions.Metadata)
		assert.Nil(intersection.Permissions.OrganizationAdministration)

		// An unspecified field.
		assert.Nil(intersection.Permissions.Emails)

		intersection, err = noPermissionsGitHubTokenPermissionGroup.Intersection(noPermissionsGitHubTokenPermissionGroup)
		require.NoError(err)
		assert.Equal(noPermissionsGitHubTokenPermissionGroup.Name, intersection.Name)
		assert.True(intersection.HasNoPermissions())

		// Specified fields.
		assert.Nil(intersection.Permissions.Administration)
		assert.Nil(intersection.Permissions.Actions)
		assert.Nil(intersection.Permissions.Contents)
		assert.Nil(intersection.Permissions.Followers)
		assert.Nil(intersection.Permissions.Checks)
		assert.Nil(intersection.Permissions.Metadata)
		assert.Nil(intersection.Permissions.OrganizationAdministration)

		// An unspecified field.
		assert.Nil(intersection.Permissions.Emails)
	})

	t.Run("Intersection of permissions that result in no permissions should return no permissions", func(t *testing.T) {
		intersection, err := orgGroup[1].Intersection(orgGroup[6])
		require.NoError(err)
		assert.True(intersection.HasNoPermissions())

		assert.Nil(intersection.Permissions.Administration)
		assert.Nil(intersection.Permissions.Actions)
		assert.Nil(intersection.Permissions.Contents)
		assert.Nil(intersection.Permissions.Followers)
		assert.Nil(intersection.Permissions.Checks)
		assert.Nil(intersection.Permissions.Metadata)
		assert.Nil(intersection.Permissions.OrganizationAdministration)
	})

	t.Run("Intersection of permissions with invalid permissions should return an error", func(t *testing.T) {
		_, err := orgGroup[0].Intersection(orgGroup[3])
		assert.ErrorContains(err, "not-a-permission")
	})

	t.Run("Intersection of permissions with all permissions should return the same values as the permissions", func(t *testing.T) {
		intersection, err := orgGroup[0].Intersection(orgGroup[4])
		require.NoError(err)

		// Fields that were set on orgGroup[0].
		assert.Equal("admin", utility.FromStringPtr(intersection.Permissions.Administration))
		assert.Equal("read", utility.FromStringPtr(intersection.Permissions.Actions))
		assert.Equal("write", utility.FromStringPtr(intersection.Permissions.Contents))
		assert.Equal("write", utility.FromStringPtr(intersection.Permissions.Checks))
		assert.Equal("write", utility.FromStringPtr(intersection.Permissions.Metadata))
		assert.Equal("admin", utility.FromStringPtr(intersection.Permissions.OrganizationAdministration))

		// An unspecified field.
		assert.Nil(intersection.Permissions.Emails)
	})

	t.Run("Intersection of two all permissions should result in all permissions", func(t *testing.T) {
		intersection, err := orgGroup[4].Intersection(orgGroup[5])
		require.NoError(err)
		assert.True(intersection.AllPermissions)
	})

	t.Run("Group defined with no permissions should return true for has no permissions", func(t *testing.T) {
		assert.True(orgGroup[2].HasNoPermissions())
	})

	t.Run("Group defined with some permissions should return false for has no permissions", func(t *testing.T) {
		assert.False(orgGroup[0].HasNoPermissions())
		assert.False(orgGroup[1].HasNoPermissions())
		assert.False(orgGroup[3].HasNoPermissions())
		assert.False(orgGroup[4].HasNoPermissions())
	})

	t.Run("No permission group should return true for has no permissions", func(t *testing.T) {
		assert.True(noPermissionsGitHubTokenPermissionGroup.HasNoPermissions())
	})
}

func TestFindOneProjectRefByRepoAndBranchWithPRTesting(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(db.CreateCollections(evergreen.ScopeCollection))

	projectRef, err := FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main", "")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "main",
		Enabled:          false,
		BatchTime:        10,
		Id:               "ident0",
		PRTestingEnabled: utility.FalsePtr(),
	}
	require.NoError(doc.Insert())

	// 1 disabled document = no match
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main", "")
	assert.NoError(err)
	assert.Nil(projectRef)

	// 2 docs, 1 enabled, but the enabled one has pr testing disabled = no match
	doc.Id = "ident_"
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.Enabled = true
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main", "")
	assert.NoError(err)
	require.Nil(projectRef)

	// 3 docs, 2 enabled, but only 1 has pr testing enabled = match
	doc.Id = "ident1"
	doc.PRTestingEnabled = utility.TruePtr()
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main", "")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("ident1", projectRef.Id)

	// 2 matching documents, we just return one of those projects
	doc.Id = "ident2"
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "main", "")
	assert.NoError(err)
	assert.NotNil(projectRef)

	repoDoc := RepoRef{ProjectRef{
		Id:         "my_repo",
		Owner:      "mongodb",
		Repo:       "mci",
		RemotePath: "",
	}}
	assert.NoError(repoDoc.Upsert())
	doc = &ProjectRef{
		Id:        "defaulting_project",
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "mine",
		Enabled:   true,
		RepoRefId: repoDoc.Id,
	}
	assert.NoError(doc.Insert())
	doc2 := &ProjectRef{
		Id:               "hidden_project",
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "mine",
		RepoRefId:        repoDoc.Id,
		Enabled:          false,
		PRTestingEnabled: utility.FalsePtr(),
		Hidden:           utility.TruePtr(),
	}
	assert.NoError(doc2.Insert())

	// repo doesn't have PR testing enabled, so no project returned
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.Nil(projectRef)

	repoDoc.PRTestingEnabled = utility.TruePtr()
	assert.NoError(repoDoc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", "")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("defaulting_project", projectRef.Id)

	// project PR testing explicitly disabled
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.ManualPRTestingEnabled = utility.FalsePtr()
	assert.NoError(doc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.Nil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", patch.AutomatedCaller)
	assert.NoError(err)
	assert.Nil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", patch.ManualCaller)
	assert.NoError(err)
	assert.Nil(projectRef)

	// project auto PR testing enabled, manual disabled
	doc.PRTestingEnabled = utility.TruePtr()
	doc.ManualPRTestingEnabled = utility.FalsePtr()
	assert.NoError(doc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.NotNil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", patch.AutomatedCaller)
	assert.NoError(err)
	assert.NotNil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", patch.ManualCaller)
	assert.NoError(err)
	assert.Nil(projectRef)

	// project auto PR testing disabled, manual enabled
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.ManualPRTestingEnabled = utility.TruePtr()
	assert.NoError(doc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.NotNil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", patch.AutomatedCaller)
	assert.NoError(err)
	assert.Nil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", patch.ManualCaller)
	assert.NoError(err)
	assert.NotNil(projectRef)

	// project explicitly disabled
	repoDoc.RemotePath = "my_path"
	assert.NoError(repoDoc.Upsert())
	doc.Enabled = false
	doc.PRTestingEnabled = utility.TruePtr()
	assert.NoError(doc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.Nil(projectRef)

	// branch with no project doesn't work and returns an error if repo not configured with a remote path
	repoDoc.RemotePath = ""
	assert.NoError(repoDoc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "yours", "")
	assert.Error(err)
	assert.Nil(projectRef)

	repoDoc.RemotePath = "my_path"
	assert.NoError(repoDoc.Upsert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "yours", "")
	assert.NoError(err)
	assert.NotNil(projectRef)
	assert.Equal("yours", projectRef.Branch)
	assert.True(projectRef.IsHidden())
	firstAttemptId := projectRef.Id

	// verify we return the same hidden project
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "yours", "")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal(firstAttemptId, projectRef.Id)
}

func TestFindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	projectRef, err := FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "mci",
		Enabled: true,
	}
	require.NoError(doc.Insert())

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc.CommitQueue.Enabled = utility.TruePtr()
	require.NoError(db.Update(ProjectRefCollection, mgobson.M{ProjectRefIdKey: "mci"}, doc))

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "main")
	assert.NoError(err)
	assert.NotNil(projectRef)
	assert.Equal("mci", projectRef.Id)

	// doc doesn't default to repo
	doc.CommitQueue.Enabled = utility.FalsePtr()
	assert.NoError(doc.Upsert())
	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "not_main")
	assert.NoError(err)
	assert.Nil(projectRef)
}

func TestValidateEnabledRepotracker(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.Clear(ProjectRefCollection))
	// A project that doesn't have repotracker enabled and an invalid config.
	p1 := &ProjectRef{
		Owner:               "mongodb",
		Repo:                "mci",
		Branch:              "main",
		Id:                  "p1",
		Enabled:             true,
		RepotrackerDisabled: utility.TruePtr(),
	}
	require.NoError(p1.Insert())
	assert.NoError(p1.ValidateEnabledRepotracker())
	// A project that doesn't have repotracker enabled and a valid config.
	p2 := &ProjectRef{
		Owner:               "mongodb",
		Repo:                "mci",
		Branch:              "main",
		Id:                  "p2",
		Enabled:             true,
		RepotrackerDisabled: utility.TruePtr(),
		RemotePath:          "valid!",
	}
	require.NoError(p2.Insert())
	assert.NoError(p2.ValidateEnabledRepotracker())
	// A project that does have repotracker enabled and a invalid config.
	p3 := &ProjectRef{
		Owner:               "mongodb",
		Repo:                "mci",
		Branch:              "main",
		Id:                  "p3",
		Enabled:             true,
		RepotrackerDisabled: utility.FalsePtr(),
	}
	require.NoError(p3.Insert())
	assert.Error(p3.ValidateEnabledRepotracker())
	// A project that does have repotracker enabled and a valid config.
	p4 := &ProjectRef{
		Owner:               "mongodb",
		Repo:                "mci",
		Branch:              "main",
		Id:                  "p4",
		Enabled:             true,
		RepotrackerDisabled: utility.FalsePtr(),
		RemotePath:          "valid!",
	}
	require.NoError(p4.Insert())
	assert.NoError(p4.ValidateEnabledRepotracker())
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
		Enabled: true,
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
		Enabled: true,
		CommitQueue: CommitQueueParams{
			Enabled: utility.FalsePtr(),
		},
	}
	require.NoError(doc2.Insert())
	ok, err = doc2.CanEnableCommitQueue()
	assert.NoError(err)
	assert.False(ok)
}

func TestFindProjectRefIdsWithCommitQueueEnabled(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))
	res, err := FindProjectRefIdsWithCommitQueueEnabled()
	assert.NoError(err)
	assert.Empty(res)

	repoRef := RepoRef{ProjectRef{
		Id: "my_repo",
		CommitQueue: CommitQueueParams{
			Enabled:    utility.TruePtr(),
			MergeQueue: MergeQueueEvergreen,
		},
	}}
	assert.NoError(repoRef.Upsert())
	doc := &ProjectRef{
		Enabled:    true,
		Owner:      "mongodb",
		Repo:       "mci",
		Branch:     "main",
		Identifier: "mci",
		Id:         "mci1",
		RepoRefId:  repoRef.Id,
		CommitQueue: CommitQueueParams{
			Enabled:    utility.TruePtr(),
			MergeQueue: MergeQueueEvergreen,
		},
	}
	require.NoError(doc.Insert())

	doc.Branch = "fix"
	doc.Id = "mci2"
	doc.CommitQueue.MergeQueue = "" // legacy behavior is unpopulated
	require.NoError(doc.Insert())

	doc.Identifier = "grip"
	doc.Repo = "grip"
	doc.Id = "mci3"
	doc.CommitQueue.Enabled = utility.FalsePtr()
	require.NoError(doc.Insert())

	doc.Identifier = "merge"
	doc.Repo = "merge"
	doc.Id = "mci4"
	doc.CommitQueue.Enabled = utility.TruePtr()
	doc.CommitQueue.MergeQueue = MergeQueueGitHub
	require.NoError(doc.Insert())

	// Should find two projects, both enabled at the branch level.
	res, err = FindProjectRefIdsWithCommitQueueEnabled()
	assert.NoError(err)
	require.Len(res, 2)
	assert.Equal("mci1", res[0])
	assert.Equal("mci2", res[1])

	// Should find three projects, because this new project defaults to repo.
	doc.Id = "commit_queue_setting_from_repo"
	doc.CommitQueue.Enabled = nil
	assert.NoError(doc.Insert())
	res, err = FindProjectRefIdsWithCommitQueueEnabled()
	assert.NoError(err)
	assert.Len(res, 3)

	// Should find two projects again now that the repo isn't enabled.
	repoRef.CommitQueue.Enabled = utility.FalsePtr()
	assert.NoError(repoRef.Upsert())
	res, err = FindProjectRefIdsWithCommitQueueEnabled()
	assert.NoError(err)
	assert.Len(res, 2)
}

func TestValidatePeriodicBuildDefinition(t *testing.T) {
	assert := assert.New(t)
	testCases := map[PeriodicBuildDefinition]bool{
		{
			IntervalHours: 24,
			ConfigFile:    "foo.yml",
			Alias:         "myAlias",
		}: true,
		{
			IntervalHours: 0,
			ConfigFile:    "foo.yml",
			Alias:         "myAlias",
		}: false,
		{
			IntervalHours: 24,
			ConfigFile:    "",
			Alias:         "myAlias",
		}: false,
		{
			IntervalHours: 24,
			ConfigFile:    "foo.yml",
			Alias:         "",
		}: true,
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

func TestContainerSecretValidate(t *testing.T) {
	t.Run("FailsWithInvalidSecretType", func(t *testing.T) {
		cs := ContainerSecret{
			Name:  "secret_name",
			Type:  "",
			Value: "new_value",
		}
		assert.Error(t, cs.Validate())
	})
	t.Run("FailsWithoutName", func(t *testing.T) {
		cs := ContainerSecret{
			Name:  "secret_name",
			Type:  ContainerSecretPodSecret,
			Value: "",
		}
		assert.Error(t, cs.Validate())
	})
	t.Run("FailsWithoutNewSecretValue", func(t *testing.T) {
		cs := ContainerSecret{
			Name:  "secret_name",
			Type:  ContainerSecretPodSecret,
			Value: "",
		}
		assert.Error(t, cs.Validate())
	})
}

func TestValidateContainerSecrets(t *testing.T) {
	var settings evergreen.Settings
	settings.Providers.AWS.Pod.SecretsManager.SecretPrefix = "secret_prefix"
	const projectID = "project_id"

	t.Run("AddsNewSecretsWithoutAnyExistingSecrets", func(t *testing.T) {
		toUpdate := []ContainerSecret{
			{
				Name:  "apple",
				Value: "new_value0",
				Type:  ContainerSecretRepoCreds,
			},
			{
				Name:  "orange",
				Value: "new_value1",
				Type:  ContainerSecretRepoCreds,
			},
		}
		combined, err := ValidateContainerSecrets(&settings, projectID, nil, toUpdate)
		require.NoError(t, err)

		require.Len(t, combined, len(toUpdate))
		for i := 0; i < len(toUpdate); i++ {
			assert.Equal(t, toUpdate[i].Name, combined[i].Name)
			assert.Equal(t, toUpdate[i].Type, combined[i].Type)
			assert.Equal(t, toUpdate[i].Value, combined[i].Value)
			assert.Zero(t, combined[i].ExternalID)
			assert.NotZero(t, combined[i].ExternalName)
		}
	})
	t.Run("IgnoresUserDefinedExternalFieldsForNewSecrets", func(t *testing.T) {
		toUpdate := []ContainerSecret{
			{
				Name:         "apple",
				ExternalName: "external_name",
				ExternalID:   "external_id",
				Value:        "new_value0",
				Type:         ContainerSecretRepoCreds,
			},
		}
		combined, err := ValidateContainerSecrets(&settings, projectID, nil, toUpdate)
		require.NoError(t, err)

		require.Len(t, combined, 1)
		assert.Equal(t, toUpdate[0].Name, combined[0].Name)
		assert.Equal(t, toUpdate[0].Type, combined[0].Type)
		assert.NotZero(t, combined[0].ExternalName)
		assert.NotEqual(t, toUpdate[0].ExternalName, combined[0].ExternalName, "external name should not be settable by users and should be generated for new secrets")
		assert.Zero(t, combined[0].ExternalID, "external ID should not be settable by users for new secrets")
	})
	t.Run("NoopsWithIdenticalOriginalAndUpdatedSecrets", func(t *testing.T) {
		secrets := []ContainerSecret{
			{
				Name:         "apple",
				ExternalName: "external_name0",
				ExternalID:   "external_id0",
				Type:         ContainerSecretRepoCreds,
			},
			{
				Name:         "orange",
				ExternalName: "external_name1",
				ExternalID:   "external_id1",
				Type:         ContainerSecretRepoCreds,
			},
		}
		combined, err := ValidateContainerSecrets(&settings, projectID, secrets, secrets)
		require.NoError(t, err)

		assert.Equal(t, combined, secrets)
	})
	t.Run("AddsNewContainerSecretsToExistingSecrets", func(t *testing.T) {
		original := []ContainerSecret{
			{
				Name:         "apple",
				ExternalName: "external_name0",
				ExternalID:   "external_id0",
				Type:         ContainerSecretRepoCreds,
			},
		}
		toUpdate := []ContainerSecret{
			{
				Name:  "orange",
				Type:  ContainerSecretRepoCreds,
				Value: "new_value",
			},
		}
		combined, err := ValidateContainerSecrets(&settings, projectID, original, toUpdate)
		require.NoError(t, err)

		require.Len(t, combined, 2)
		assert.Equal(t, original[0], combined[0])
		assert.Equal(t, toUpdate[0].Name, combined[1].Name)
		assert.Equal(t, toUpdate[0].Type, combined[1].Type)
		assert.Equal(t, toUpdate[0].Value, combined[1].Value)
		assert.NotZero(t, combined[1].ExternalName)
		assert.Zero(t, combined[1].ExternalID)
	})
	t.Run("SetsUpdatedValueForExistingSecret", func(t *testing.T) {
		original := []ContainerSecret{
			{
				Name:         "pineapple",
				ExternalName: "a_legit_pizza_topping",
				ExternalID:   "external_id",
				Type:         ContainerSecretPodSecret,
			},
		}
		toUpdate := []ContainerSecret{
			{
				Name:  "pineapple",
				Value: "new_value",
			},
		}
		combined, err := ValidateContainerSecrets(&settings, projectID, original, toUpdate)
		require.NoError(t, err)

		require.Len(t, combined, 1)
		assert.Equal(t, original[0].Name, combined[0].Name)
		assert.Equal(t, original[0].ExternalName, combined[0].ExternalName)
		assert.Equal(t, original[0].ExternalID, combined[0].ExternalID)
		assert.Equal(t, original[0].Type, combined[0].Type)
		assert.Equal(t, toUpdate[0].Value, combined[0].Value)
	})
	t.Run("CombinesExistingSecretsAndUpdatedSecrets", func(t *testing.T) {
		original := []ContainerSecret{
			{
				Name:         "apple",
				ExternalName: "external_name0",
				ExternalID:   "external_id0",
				Type:         ContainerSecretPodSecret,
			},
			{
				Name:         "banana",
				ExternalName: "external_name1",
				ExternalID:   "external_id1",
				Type:         ContainerSecretRepoCreds,
			},
		}
		updated := []ContainerSecret{
			{
				Name:  "cherry",
				Value: "new_value0",
				Type:  ContainerSecretRepoCreds,
			},
			{
				Name:         "banana",
				ExternalName: "external_name1",
				ExternalID:   "external_id1",
				Value:        "new_value1",
				Type:         ContainerSecretRepoCreds,
			},
		}
		combined, err := ValidateContainerSecrets(&settings, projectID, original, updated)
		require.NoError(t, err)

		require.Len(t, combined, 3)
		assert.Equal(t, original[0], combined[0])
		assert.Equal(t, original[1].Name, combined[1].Name)
		assert.Equal(t, original[1].ExternalName, combined[1].ExternalName)
		assert.Equal(t, original[1].ExternalID, combined[1].ExternalID)
		assert.Equal(t, original[1].Type, combined[1].Type)
		assert.Equal(t, updated[1].Value, combined[1].Value)
		assert.Equal(t, updated[0].Name, combined[2].Name)
		assert.NotZero(t, combined[2].ExternalName)
		assert.Zero(t, combined[2].ExternalID)
		assert.Equal(t, updated[0].Type, combined[2].Type)
		assert.Equal(t, updated[0].Value, combined[2].Value)
	})
	t.Run("ReturnsOriginalForNoUpdatedSecrets", func(t *testing.T) {
		original := []ContainerSecret{
			{
				Name:         "apple",
				ExternalName: "external_name0",
				ExternalID:   "external_id0",
				Type:         ContainerSecretPodSecret,
			},
			{
				Name:         "banana",
				ExternalName: "external_name1",
				ExternalID:   "external_id1",
				Type:         ContainerSecretRepoCreds,
			},
		}
		combined, err := ValidateContainerSecrets(&settings, projectID, original, nil)
		assert.NoError(t, err)
		assert.Equal(t, original, combined)
	})
	t.Run("ReturnsEmptyWithoutAnyExistingOrUpdatedSecrets", func(t *testing.T) {
		secrets, err := ValidateContainerSecrets(&settings, projectID, nil, nil)
		assert.NoError(t, err)
		assert.Empty(t, secrets)
	})
	t.Run("FailsWithInvalidSecretType", func(t *testing.T) {
		toUpdate := []ContainerSecret{
			{
				Name: "breadfruit",
				Type: "a type of bread",
			},
		}
		_, err := ValidateContainerSecrets(&settings, projectID, nil, toUpdate)
		assert.Error(t, err)
	})
	t.Run("FailsWithDifferentTypeForExistingSecret", func(t *testing.T) {
		original := []ContainerSecret{
			{
				Name:         "starfruit",
				ExternalName: "external_name",
				ExternalID:   "external_id",
				Type:         ContainerSecretRepoCreds,
			},
		}
		toUpdate := []ContainerSecret{
			{
				Name:         "starfruit",
				ExternalName: "external_name",
				ExternalID:   "external_id",
				Type:         ContainerSecretPodSecret,
			},
		}
		_, err := ValidateContainerSecrets(&settings, projectID, original, toUpdate)
		assert.Error(t, err)
	})
	t.Run("FailsWithDifferentExternalNameForExistingSecret", func(t *testing.T) {
		original := []ContainerSecret{
			{
				Name:         "starfruit",
				ExternalID:   "external_id",
				ExternalName: "a_starfruit",
				Type:         ContainerSecretRepoCreds,
			},
		}
		toUpdate := []ContainerSecret{
			{
				Name:         "starfruit",
				ExternalID:   "external_id",
				ExternalName: "not_a_starfruit_no_more",
				Type:         ContainerSecretRepoCreds,
			},
		}
		_, err := ValidateContainerSecrets(&settings, projectID, original, toUpdate)
		assert.Error(t, err)
	})
	t.Run("FailsWithDifferentExternalIDForExistingSecret", func(t *testing.T) {
		original := []ContainerSecret{
			{
				Name:         "starfruit",
				ExternalID:   "a_starfruit",
				ExternalName: "external_name",
				Type:         ContainerSecretRepoCreds,
			},
		}
		toUpdate := []ContainerSecret{
			{
				Name:         "starfruit",
				ExternalID:   "not_a_starfruit_no_more",
				ExternalName: "external_name",
				Type:         ContainerSecretRepoCreds,
			},
		}
		_, err := ValidateContainerSecrets(&settings, projectID, original, toUpdate)
		assert.Error(t, err)
	})
	t.Run("FailsWithoutName", func(t *testing.T) {
		containerSecrets := []ContainerSecret{
			{
				Type:  ContainerSecretPodSecret,
				Value: "value",
			},
		}
		_, err := ValidateContainerSecrets(&settings, projectID, nil, containerSecrets)
		assert.Error(t, err)
	})
	t.Run("FailsWithMultiplePodSecrets", func(t *testing.T) {
		toUpdate := []ContainerSecret{
			{
				Name:  "breadfruit",
				Type:  ContainerSecretPodSecret,
				Value: "abcde",
			},
			{
				Name:  "starfruit",
				Type:  ContainerSecretPodSecret,
				Value: "12345",
			},
		}
		_, err := ValidateContainerSecrets(&settings, projectID, nil, toUpdate)
		assert.Error(t, err)

		toUpdate = []ContainerSecret{
			{
				Name:  "pear",
				Type:  ContainerSecretPodSecret,
				Value: "abcde",
			},
		}
		original := []ContainerSecret{
			{
				Name:  "dragonfruit",
				Type:  ContainerSecretPodSecret,
				Value: "abcde",
			},
		}
		_, err = ValidateContainerSecrets(&settings, projectID, original, toUpdate)
		assert.Error(t, err)
	})
}

func TestContainerSecretCache(t *testing.T) {
	assert.Implements(t, (*cocoa.SecretCache)(nil), ContainerSecretCache{})
	defer func() {
		assert.NoError(t, db.ClearCollections(ProjectRefCollection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache){
		"PutSucceeds": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			pRef.ContainerSecrets[0].ExternalID = ""
			require.NoError(t, pRef.Insert())
			const externalID = "external_id"
			require.NoError(t, c.Put(ctx, cocoa.SecretCacheItem{
				ID:   externalID,
				Name: pRef.ContainerSecrets[0].ExternalName,
			}))

			dbProjRef, err := FindMergedProjectRef(pRef.Id, "", false)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			require.Len(t, dbProjRef.ContainerSecrets, len(pRef.ContainerSecrets))
			original := pRef.ContainerSecrets[0]
			updated := dbProjRef.ContainerSecrets[0]
			assert.Equal(t, original.ExternalName, updated.ExternalName)
			assert.Equal(t, original.Name, updated.Name)
			assert.Equal(t, original.Type, updated.Type)
			assert.Equal(t, externalID, updated.ExternalID)
			for i := 1; i < len(pRef.ContainerSecrets); i++ {
				assert.Equal(t, pRef.ContainerSecrets[i], dbProjRef.ContainerSecrets[i], "mismatched container secrets at index %d", i)
			}
		},
		"PutFailsWithNonexistentProjectRef": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			assert.Error(t, c.Put(ctx, cocoa.SecretCacheItem{ID: "external_id", Name: pRef.ContainerSecrets[0].ExternalName}))
		},
		"PutFailsWithoutMatchingContainerSecretExternalName": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			require.NoError(t, pRef.Insert())
			assert.Error(t, c.Put(ctx, cocoa.SecretCacheItem{
				ID:   "external_id",
				Name: "nonexistent",
			}))

			dbProjRef, err := FindMergedProjectRef(pRef.Id, "", false)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			require.Len(t, dbProjRef.ContainerSecrets, len(pRef.ContainerSecrets))
			for i := 0; i < len(pRef.ContainerSecrets); i++ {
				assert.Equal(t, pRef.ContainerSecrets[i], dbProjRef.ContainerSecrets[i], "mismatched container secrets at index %d", i)
			}
		},
		"PutSucceedsWithContainerSecretThatAlreadyHasSameExternalIDAlreadySet": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			pRef.ContainerSecrets[0].ExternalID = "external_id"
			require.NoError(t, pRef.Insert())
			require.NoError(t, c.Put(ctx, cocoa.SecretCacheItem{
				ID:   pRef.ContainerSecrets[0].ExternalID,
				Name: pRef.ContainerSecrets[0].ExternalName,
			}))

			dbProjRef, err := FindMergedProjectRef(pRef.Id, "", false)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			require.Len(t, dbProjRef.ContainerSecrets, len(pRef.ContainerSecrets))
			for i := 0; i < len(pRef.ContainerSecrets); i++ {
				assert.Equal(t, pRef.ContainerSecrets[i], dbProjRef.ContainerSecrets[i], "mismatched container secrets at index %d", i)
			}
		},
		"PutFailsWithContainerSecretThatHasDifferentExternalIDAlreadySet": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			const externalID = "external_id"
			pRef.ContainerSecrets[0].ExternalID = "something_else"
			require.NoError(t, pRef.Insert())
			require.Error(t, c.Put(ctx, cocoa.SecretCacheItem{
				ID:   externalID,
				Name: pRef.ContainerSecrets[0].ExternalName,
			}))

			dbProjRef, err := FindMergedProjectRef(pRef.Id, "", false)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			require.Len(t, dbProjRef.ContainerSecrets, len(pRef.ContainerSecrets))
			for i := 0; i < len(pRef.ContainerSecrets); i++ {
				assert.Equal(t, pRef.ContainerSecrets[i], dbProjRef.ContainerSecrets[i], "mismatched container secrets at index %d", i)
			}
		},
		"DeleteSucceeds": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			require.NoError(t, pRef.Insert())
			require.NoError(t, c.Delete(ctx, pRef.ContainerSecrets[1].ExternalID))

			dbProjRef, err := FindMergedProjectRef(pRef.Id, "", false)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			require.Len(t, dbProjRef.ContainerSecrets, len(pRef.ContainerSecrets)-1)
			assert.Equal(t, dbProjRef.ContainerSecrets[0], pRef.ContainerSecrets[0])
		},
		"DeleteNoopsWithNonexistentProjectRef": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			assert.NoError(t, c.Delete(ctx, "external_id"), "should not for nonexistent project ref")
			assert.True(t, adb.ResultsNotFound(db.FindOneQ(ProjectRefCollection, db.Query(bson.M{}), &pRef)))
		},
		"DeleteNoopsWithoutMatchingContainerSecretExternalID": func(ctx context.Context, t *testing.T, pRef ProjectRef, c ContainerSecretCache) {
			require.NoError(t, pRef.Insert())
			assert.NoError(t, c.Delete(ctx, "nonexistent"), "should not error for nonexistent container secret")

			dbProjRef, err := FindMergedProjectRef(pRef.Id, "", false)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			assert.Len(t, dbProjRef.ContainerSecrets, len(pRef.ContainerSecrets))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(ProjectRefCollection))
			pRef := ProjectRef{
				Id:         "project_id",
				Identifier: "identifier",
				ContainerSecrets: []ContainerSecret{
					{
						Name:       "banana",
						Type:       ContainerSecretRepoCreds,
						ExternalID: "external_id0",
					},
					{
						Name:       "cherry",
						Type:       ContainerSecretRepoCreds,
						ExternalID: "external_id1",
					},
					{
						Name:       "banerry",
						Type:       ContainerSecretRepoCreds,
						ExternalID: "external_id2",
					},
				},
			}
			for i := 0; i < len(pRef.ContainerSecrets); i++ {
				pRef.ContainerSecrets[i].ExternalName = makeRepoCredsContainerSecretName(evergreen.SecretsManagerConfig{
					SecretPrefix: "prefix",
				}, pRef.Id, pRef.ContainerSecrets[i].Name)
			}

			tCase(ctx, t, pRef, ContainerSecretCache{})
		})
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
	require.NoError(t, db.ClearCollections(ProjectRefCollection))

	proj1 := ProjectRef{
		Id:       "evergreen",
		Enabled:  true,
		Triggers: []TriggerDefinition{{Project: "grip"}},
	}
	require.NoError(t, proj1.Insert())

	proj2 := ProjectRef{
		Id:       "mci",
		Enabled:  false,
		Triggers: []TriggerDefinition{{Project: "grip"}},
	}
	require.NoError(t, proj2.Insert())

	projects, err := FindDownstreamProjects("grip")
	assert.NoError(t, err)
	assert.Len(t, projects, 1)
	assert.Equal(t, proj1, projects[0])
}

func TestAddEmptyBranch(t *testing.T) {
	require.NoError(t, db.ClearCollections(user.Collection, ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, commitqueue.Collection))
	u := user.DBUser{
		Id: "me",
	}
	require.NoError(t, u.Insert())
	p := ProjectRef{
		Identifier: "myProject",
		Owner:      "mongodb",
		Repo:       "mongo",
	}
	assert.NoError(t, p.Add(&u))
	assert.NotEmpty(t, p.Id)
	assert.Empty(t, p.Branch)

	cq, err := commitqueue.FindOneId(p.Id)
	assert.NoError(t, err)
	assert.NotNil(t, cq)
}

func TestAddPermissions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(user.Collection, ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, commitqueue.Collection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	env := testutil.NewEnvironment(ctx, t)
	u := user.DBUser{
		Id: "me",
	}
	assert.NoError(u.Insert())
	p := ProjectRef{
		Identifier: "myProject",
		Owner:      "mongodb",
		Repo:       "mongo",
		Branch:     "main",
		Hidden:     utility.TruePtr(),
	}
	assert.NoError(p.Add(&u))
	assert.NotEmpty(p.Id)
	assert.True(mgobson.IsObjectIdHex(p.Id))

	cq, err := commitqueue.FindOneId(p.Id)
	assert.NoError(err)
	assert.NotNil(cq)

	rm := env.RoleManager()
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
	projectId := p.Id

	// check that an added project uses the hidden project's ID
	u = user.DBUser{Id: "you"}
	assert.NoError(u.Insert())
	p.Identifier = "differentProject"
	p.Id = ""
	assert.NoError(p.Add(&u))
	assert.NotEmpty(p.Id)
	assert.True(mgobson.IsObjectIdHex(p.Id))
	assert.Equal(projectId, p.Id)

	cq, err = commitqueue.FindOneId(p.Id)
	assert.NoError(err)
	assert.NotNil(cq)

	scope, err = rm.FindScopeForResources(evergreen.ProjectResourceType, p.Id)
	assert.NoError(err)
	assert.NotNil(scope)
	role, err = rm.FindRoleWithPermissions(evergreen.ProjectResourceType, []string{p.Id}, map[string]int{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
		evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
		evergreen.PermissionLogs:            evergreen.LogsView.Value,
	})
	assert.NoError(err)
	assert.NotNil(role)
	dbUser, err = user.FindOneById(u.Id)
	assert.NoError(err)
	assert.Contains(dbUser.Roles(), fmt.Sprintf("admin_project_%s", p.Id))
}

func TestUpdateAdminRoles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	env := testutil.NewEnvironment(ctx, t)
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

	modified, err := p.UpdateAdminRoles([]string{newAdmin.Id}, []string{oldAdmin.Id})
	assert.NoError(t, err)
	assert.True(t, modified)
	oldAdminFromDB, err := user.FindOneById(oldAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, oldAdminFromDB.Roles(), 0)
	newAdminFromDB, err := user.FindOneById(newAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, newAdminFromDB.Roles(), 1)
}

func TestUpdateAdminRolesError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection, user.Collection))
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
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
		Id:     "proj",
		Admins: []string{oldAdmin.Id},
	}
	require.NoError(t, p.Insert())

	// check that, without a valid role, the whole update fails
	modified, err := p.UpdateAdminRoles([]string{"nonexistent-user", newAdmin.Id}, []string{"nonexistent-user", oldAdmin.Id})
	assert.Error(t, err)
	assert.False(t, modified)
	assert.Equal(t, p.Admins, []string{oldAdmin.Id})

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

	// check that the existing users have been added and removed while returning an error
	modified, err = p.UpdateAdminRoles([]string{"nonexistent-user", newAdmin.Id}, []string{"nonexistent-user", oldAdmin.Id})
	assert.Error(t, err)
	assert.True(t, modified)
	oldAdminFromDB, err := user.FindOneById(oldAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, oldAdminFromDB.Roles(), 0)
	newAdminFromDB, err := user.FindOneById(newAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, newAdminFromDB.Roles(), 1)
}

func TestGetProjectTasksWithOptions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection, ProjectRefCollection, RepositoriesCollection))
	p := ProjectRef{
		Id:         "my_project",
		Identifier: "my_ident",
	}
	assert.NoError(t, p.Insert())
	assert.NoError(t, db.Insert(RepositoriesCollection, Repository{
		Project:             "my_project",
		RevisionOrderNumber: 100,
	}))

	// total of 100 tasks eligible to be found
	for i := 0; i < 100; i++ {
		myTask := task.Task{
			Id:                  fmt.Sprintf("t%d", i),
			RevisionOrderNumber: 100 - (i / 2),
			DisplayName:         "t1",
			Project:             "my_project",
			Status:              evergreen.TaskSucceeded,
			Version:             fmt.Sprintf("v%d", 100-(i/2)),
		}
		if i%3 == 0 {
			myTask.BuildVariant = "bv1"
			myTask.Requester = evergreen.RepotrackerVersionRequester
		} else {
			myTask.Requester = evergreen.PatchVersionRequester
		}
		if i%2 == 0 {
			myTask.Status = evergreen.TaskUndispatched
		}
		assert.NoError(t, myTask.Insert())
	}
	opts := GetProjectTasksOpts{}

	tasks, err := GetTasksWithOptions("my_ident", "t1", opts)
	assert.NoError(t, err)
	// Returns 7 tasks because 40 tasks exist within the default version limit,
	// but 1/2 are undispatched and only 1/3 have a system requester
	assert.Len(t, tasks, 7)

	opts.Limit = 5
	tasks, err = GetTasksWithOptions("my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.Equal(t, tasks[0].RevisionOrderNumber, 99)
	assert.Equal(t, tasks[1].RevisionOrderNumber, 96)

	opts.Limit = 10
	opts.StartAt = 80
	tasks, err = GetTasksWithOptions("my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 3)
	assert.Equal(t, tasks[0].RevisionOrderNumber, 78)
	assert.Equal(t, tasks[2].RevisionOrderNumber, 72)

	opts.Requesters = []string{evergreen.PatchVersionRequester}
	tasks, err = GetTasksWithOptions("my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 7)
	assert.Equal(t, tasks[0].RevisionOrderNumber, 80)
	assert.Equal(t, tasks[6].RevisionOrderNumber, 71)

	opts.Requesters = []string{evergreen.RepotrackerVersionRequester}
	tasks, err = GetTasksWithOptions("my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 3)
	assert.Equal(t, tasks[0].RevisionOrderNumber, 78)
	assert.Equal(t, tasks[2].RevisionOrderNumber, 72)

	opts.Requesters = []string{}
	opts.Limit = defaultVersionLimit
	opts.StartAt = 90
	opts.BuildVariant = "bv1"
	tasks, err = GetTasksWithOptions("my_ident", "t1", opts)
	// Returns 7 tasks because 40 tasks exist within the default version limit,
	// but only 1/6 matches the bv and is not undispatched
	assert.NoError(t, err)
	assert.Len(t, tasks, 7)
	assert.Equal(t, tasks[0].RevisionOrderNumber, 90)
	assert.Equal(t, tasks[6].RevisionOrderNumber, 72)
}

func TestUpdateNextPeriodicBuild(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	now := time.Now().Truncate(time.Second)
	later := now.Add(1 * time.Hour)
	muchLater := now.Add(10 * time.Hour)
	for name, test := range map[string]func(*testing.T){
		"updatesProjectOnly": func(t *testing.T) {
			p := ProjectRef{
				Id: "proj",
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "0", NextRunTime: now},
					{ID: "1", NextRunTime: later, IntervalHours: 9},
				},
				RepoRefId: "repo",
			}
			repoRef := RepoRef{ProjectRef{
				Id: "repo",
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "1", NextRunTime: later},
				},
			}}
			assert.NoError(p.Insert())
			assert.NoError(repoRef.Upsert())

			assert.NoError(UpdateNextPeriodicBuild("proj", &p.PeriodicBuilds[1]))
			dbProject, err := FindBranchProjectRef(p.Id)
			assert.NoError(err)
			require.NotNil(dbProject)
			assert.True(now.Equal(dbProject.PeriodicBuilds[0].NextRunTime))
			assert.True(muchLater.Equal(dbProject.PeriodicBuilds[1].NextRunTime))

			dbRepo, err := FindOneRepoRef(p.RepoRefId)
			assert.NoError(err)
			require.NotNil(dbRepo)
			// Repo wasn't updated because the branch project definitions take precedent.
			assert.True(later.Equal(dbRepo.PeriodicBuilds[0].NextRunTime))
		},
		"updatesRepoOnly": func(t *testing.T) {
			p := ProjectRef{
				Id:             "proj",
				PeriodicBuilds: nil,
				RepoRefId:      "repo",
			}
			repoRef := RepoRef{ProjectRef{
				Id: "repo",
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "0", NextRunTime: later, IntervalHours: 9},
				},
			}}
			assert.NoError(p.Insert())
			assert.NoError(repoRef.Upsert())
			assert.NoError(UpdateNextPeriodicBuild("proj", &repoRef.PeriodicBuilds[0]))

			// Repo is updated because the branch project doesn't have any periodic build override defined.
			dbRepo, err := FindOneRepoRef(p.RepoRefId)
			assert.NoError(err)
			require.NotNil(dbRepo)
			assert.True(muchLater.Equal(dbRepo.PeriodicBuilds[0].NextRunTime))
		},
		"updatesNothing": func(t *testing.T) {
			p := ProjectRef{
				Id:             "proj",
				PeriodicBuilds: []PeriodicBuildDefinition{},
				RepoRefId:      "repo",
			}
			repoRef := RepoRef{ProjectRef{
				Id: "repo",
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "0", NextRunTime: later, IntervalHours: 9},
				},
			}}
			assert.NoError(p.Insert())
			assert.NoError(repoRef.Upsert())
			// Should error because definition isn't relevant for this project, since
			// we ignore repo definitions when the project has any override defined.
			assert.Error(UpdateNextPeriodicBuild("proj", &repoRef.PeriodicBuilds[0]))

			dbRepo, err := FindOneRepoRef(p.RepoRefId)
			assert.NoError(err)
			assert.NotNil(dbRepo)
			assert.True(later.Equal(dbRepo.PeriodicBuilds[0].NextRunTime))
		},
		"updateProjectWithCron": func(t *testing.T) {
			nextRunTime := time.Date(2022, 12, 12, 0, 0, 0, 0, time.UTC)
			nextDay := nextRunTime.Add(24 * time.Hour)
			laterRunTime := nextRunTime.Add(12 * time.Hour)
			dailyCron := "0 0 * * *"
			p := ProjectRef{
				Id: "proj",
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "0", NextRunTime: nextRunTime, Cron: dailyCron},
					{ID: "1", NextRunTime: laterRunTime, Cron: dailyCron},
				},
				RepoRefId: "repo",
			}
			repoRef := RepoRef{ProjectRef{
				Id: "repo",
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "1", NextRunTime: later},
				},
			}}
			assert.NoError(p.Insert())
			assert.NoError(repoRef.Upsert())

			assert.NoError(UpdateNextPeriodicBuild("proj", &p.PeriodicBuilds[0]))
			dbProject, err := FindBranchProjectRef(p.Id)
			assert.NoError(err)
			require.NotNil(dbProject)
			assert.True(nextDay.Equal(dbProject.PeriodicBuilds[0].NextRunTime))
			assert.True(laterRunTime.Equal(dbProject.PeriodicBuilds[1].NextRunTime))

			// Even with a different runtime we get the same result, since we're using a cron.
			assert.NoError(UpdateNextPeriodicBuild("proj", &p.PeriodicBuilds[1]))
			dbProject, err = FindBranchProjectRef(p.Id)
			assert.NoError(err)
			require.NotNil(dbProject)
			assert.True(nextDay.Equal(dbProject.PeriodicBuilds[1].NextRunTime))

		},
	} {
		assert.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))
		t.Run(name, test)
	}

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

func TestFindFirstProjectRef(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection))

	var err error
	projectRef := ProjectRef{
		Id:         "restricted",
		Restricted: utility.TruePtr(),
		Enabled:    true,
	}
	assert.NoError(t, projectRef.Insert())

	assert.NotPanics(t, func() {
		_, err = FindAnyRestrictedProjectRef()
	}, "Should not panic if there are no matching projects")
	assert.Error(t, err, "Should return error if there are no matching projects")

	projectRef = ProjectRef{
		Id:      "p1",
		Enabled: true,
	}
	assert.NoError(t, projectRef.Insert())

	resultRef, err := FindAnyRestrictedProjectRef()
	assert.NoError(t, err)
	assert.Equal(t, "p1", resultRef.Id)
}

func TestFindPeriodicProjects(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	repoRef := RepoRef{ProjectRef{
		Id:             "my_repo",
		PeriodicBuilds: []PeriodicBuildDefinition{{ID: "repo_def"}},
	}}
	assert.NoError(t, repoRef.Upsert())

	pRef := ProjectRef{
		Id:             "p1",
		RepoRefId:      "my_repo",
		Enabled:        true,
		PeriodicBuilds: []PeriodicBuildDefinition{},
	}
	assert.NoError(t, pRef.Insert())

	pRef.Id = "p2"
	pRef.Enabled = true
	pRef.PeriodicBuilds = []PeriodicBuildDefinition{{ID: "p1"}}
	assert.NoError(t, pRef.Insert())

	pRef.Id = "p3"
	pRef.Enabled = true
	pRef.PeriodicBuilds = nil
	assert.NoError(t, pRef.Insert())

	pRef.Id = "p4"
	pRef.Enabled = false
	pRef.PeriodicBuilds = []PeriodicBuildDefinition{{ID: "p1"}}
	assert.NoError(t, pRef.Insert())

	projects, err := FindPeriodicProjects()
	assert.NoError(t, err)
	assert.Len(t, projects, 2)
	for _, p := range projects {
		assert.Len(t, p.PeriodicBuilds, 1, fmt.Sprintf("project '%s' missing definition", p.Id))
	}
}

func TestRemoveAdminFromProjects(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	pRef := ProjectRef{
		Id:     "my_project",
		Admins: []string{"me", "villain"},
	}
	pRef2 := ProjectRef{
		Id:     "your_project",
		Admins: []string{"you", "villain"},
	}
	pRef3 := ProjectRef{
		Id: "adminless_project",
	}
	repoRef := RepoRef{ProjectRef{
		Id:     "my_repo",
		Admins: []string{"villain"},
	}}
	repoRef2 := RepoRef{ProjectRef{
		Id:     "your_repo",
		Admins: []string{"villain"},
	}}
	repoRef3 := RepoRef{ProjectRef{
		Id: "adminless_repo",
	}}

	assert.NoError(t, pRef.Upsert())
	assert.NoError(t, pRef2.Upsert())
	assert.NoError(t, pRef3.Upsert())
	assert.NoError(t, repoRef.Upsert())
	assert.NoError(t, repoRef2.Upsert())
	assert.NoError(t, repoRef3.Upsert())

	assert.NoError(t, RemoveAdminFromProjects("villain"))

	// verify that we carry out multiple updates
	pRefFromDB, err := FindBranchProjectRef(pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")
	pRefFromDB, err = FindBranchProjectRef(pRef2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")
	pRefFromDB, err = FindBranchProjectRef(pRef3.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")

	repoRefFromDB, err := FindOneRepoRef(repoRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
	repoRefFromDB, err = FindOneRepoRef(repoRef2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
	repoRefFromDB, err = FindOneRepoRef(repoRef3.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
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
		MyStruct: WorkstationConfig{GitClone: utility.TruePtr()},
	}

	assert.NoError(t, db.Insert(ProjectRefCollection, ref))

	pointerRef := struct {
		PtrString *string            `bson:"my_str"`
		PtrBool   *bool              `bson:"my_bool"`
		PtrStruct *WorkstationConfig `bson:"config"`
	}{}
	assert.NoError(t, db.FindOneQ(ProjectRefCollection, db.Query(bson.M{}), &pointerRef))
	assert.Equal(t, ref.MyString, *pointerRef.PtrString)
	assert.False(t, utility.FromBoolTPtr(pointerRef.PtrBool))
	assert.NotNil(t, pointerRef.PtrStruct)
	assert.True(t, pointerRef.PtrStruct.ShouldGitClone())
}

func TestMergeWithProjectConfig(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, ProjectConfigCollection))

	projectRef := &ProjectRef{
		Owner:              "mongodb",
		Id:                 "ident",
		DeactivatePrevious: utility.FalsePtr(),
		TaskAnnotationSettings: evergreen.AnnotationsSettings{
			FileTicketWebhook: evergreen.WebHook{
				Endpoint: "random1",
			},
		},
		WorkstationConfig: WorkstationConfig{
			GitClone: utility.TruePtr(),
			SetupCommands: []WorkstationSetupCommand{
				{Command: "expeliarmus"},
			},
		},
		BuildBaronSettings: evergreen.BuildBaronSettings{
			TicketCreateProject:  "EVG",
			TicketSearchProjects: []string{"BF", "BFG"},
		},
		PeriodicBuilds: []PeriodicBuildDefinition{{ID: "p1"}},
	}
	projectConfig := &ProjectConfig{
		Id: "version1",
		ProjectConfigFields: ProjectConfigFields{
			TaskAnnotationSettings: &evergreen.AnnotationsSettings{
				FileTicketWebhook: evergreen.WebHook{
					Endpoint: "random2",
				},
			},
			WorkstationConfig: &WorkstationConfig{
				GitClone: utility.FalsePtr(),
				SetupCommands: []WorkstationSetupCommand{
					{Command: "overridden"},
				},
			},
			ContainerSizeDefinitions: []ContainerResources{
				{
					Name:     "small",
					CPU:      1,
					MemoryMB: 200,
				},
				{
					Name:     "large",
					CPU:      2,
					MemoryMB: 400,
				},
			},
			BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketCreateIssueType:   "Bug",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			},
			GithubTriggerAliases: []string{"one", "two"},
		},
	}
	assert.NoError(t, projectRef.Insert())
	assert.NoError(t, projectConfig.Insert())

	err := projectRef.MergeWithProjectConfig("version1")
	assert.NoError(t, err)
	require.NotNil(t, projectRef)
	assert.Equal(t, "ident", projectRef.Id)

	assert.Equal(t, "random1", projectRef.TaskAnnotationSettings.FileTicketWebhook.Endpoint)
	assert.True(t, *projectRef.WorkstationConfig.GitClone)
	assert.Equal(t, "expeliarmus", projectRef.WorkstationConfig.SetupCommands[0].Command)

	assert.Equal(t, "https://evergreen.mongodb.com", projectRef.BuildBaronSettings.BFSuggestionServer)
	assert.Equal(t, 10, projectRef.BuildBaronSettings.BFSuggestionTimeoutSecs)
	assert.Equal(t, "EVG", projectRef.BuildBaronSettings.TicketCreateProject)
	assert.Equal(t, "Bug", projectRef.BuildBaronSettings.TicketCreateIssueType)
	assert.Equal(t, []string{"BF", "BFG"}, projectRef.BuildBaronSettings.TicketSearchProjects)
	assert.Equal(t, []string{"one", "two"}, projectRef.GithubTriggerAliases)
	assert.Equal(t, "p1", projectRef.PeriodicBuilds[0].ID)
	assert.Equal(t, 1, projectRef.ContainerSizeDefinitions[0].CPU)
	assert.Equal(t, 2, projectRef.ContainerSizeDefinitions[1].CPU)

	projectRef.ContainerSizeDefinitions = []ContainerResources{
		{
			Name:     "xlarge",
			CPU:      4,
			MemoryMB: 800,
		},
	}
	err = projectRef.MergeWithProjectConfig("version1")
	assert.NoError(t, err)
	require.NotNil(t, projectRef)
	assert.Equal(t, 4, projectRef.ContainerSizeDefinitions[0].CPU)
}

func TestSaveProjectPageForSection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	assert.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ConfigCollection))

	projectRef := &ProjectRef{
		Owner:            "evergreen-ci",
		Repo:             "mci",
		Branch:           "main",
		Enabled:          true,
		BatchTime:        10,
		Id:               "iden_",
		Identifier:       "identifier",
		PRTestingEnabled: utility.TruePtr(),
	}
	assert.NoError(projectRef.Insert())
	projectRef, err := FindBranchProjectRef("identifier")
	assert.NoError(err)
	assert.NotNil(t, projectRef)

	settings := evergreen.Settings{
		GithubOrgs: []string{"newOwner", "evergreen-ci"},
	}
	assert.NoError(settings.Set(ctx))

	update := &ProjectRef{
		Id:      "iden_",
		Enabled: true,
		Owner:   "evergreen-ci",
		Repo:    "test",
	}
	_, err = SaveProjectPageForSection("iden_", update, ProjectPageGeneralSection, false)
	assert.NoError(err)

	// Verify that Parsley filters and project health view are saved correctly.
	update = &ProjectRef{
		ParsleyFilters: []parsley.Filter{
			{Expression: "filter", CaseSensitive: false, ExactMatch: true},
		},
		ProjectHealthView: ProjectHealthViewAll,
	}
	_, err = SaveProjectPageForSection("iden_", update, ProjectPageViewsAndFiltersSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef("iden_")
	assert.NoError(err)
	require.NotNil(t, projectRef)
	assert.Len(projectRef.ParsleyFilters, 1)
	assert.Equal(projectRef.ProjectHealthView, ProjectHealthViewAll)

	// Verify that private field does not get updated when updating restricted field.
	update = &ProjectRef{
		Restricted: utility.TruePtr(),
	}
	_, err = SaveProjectPageForSection("iden_", update, ProjectPageAccessSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef("iden_")
	assert.NoError(err)
	require.NotNil(t, projectRef)
	assert.True(utility.FromBoolPtr(projectRef.Restricted))

	// Verify that GitHub dynamic token permission groups are saved correctly.
	update = &ProjectRef{
		GitHubDynamicTokenPermissionGroups: []GitHubDynamicTokenPermissionGroup{
			{
				Name: "some-group",
				Permissions: github.InstallationPermissions{
					Actions: utility.ToStringPtr("read"),
				},
			},
		},
	}
	_, err = SaveProjectPageForSection("iden_", update, ProjectPageGithubPermissionsSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef("iden_")
	assert.NoError(err)
	require.NotNil(t, projectRef)
	require.Len(t, projectRef.GitHubDynamicTokenPermissionGroups, 1)
	assert.Equal("some-group", projectRef.GitHubDynamicTokenPermissionGroups[0].Name)
	assert.Equal("read", utility.FromStringPtr(projectRef.GitHubDynamicTokenPermissionGroups[0].Permissions.Actions))

	// Verify that GitHub permission group by requester is saved correctly.
	update = &ProjectRef{
		GitHubPermissionGroupByRequester: map[string]string{
			evergreen.PatchVersionRequester: "some-group",
		},
	}
	_, err = SaveProjectPageForSection("iden_", update, ProjectPageGithubAppSettingsSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef("iden_")
	require.NoError(t, err)
	require.NotNil(t, projectRef)
	require.NotNil(t, projectRef.GitHubPermissionGroupByRequester)
	assert.Equal(len(projectRef.GitHubPermissionGroupByRequester), 1)
	assert.Equal(projectRef.GitHubPermissionGroupByRequester[evergreen.PatchVersionRequester], "some-group")
}

func TestValidateOwnerAndRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ConfigCollection))

	settings := evergreen.Settings{
		GithubOrgs: []string{"newOwner", "evergreen-ci"},
	}
	assert.NoError(t, settings.Set(ctx))

	// a project with no owner should error
	project := ProjectRef{
		Id:      "project",
		Enabled: true,
		Repo:    "repo",
	}
	require.NoError(t, project.Insert())

	err := project.ValidateOwnerAndRepo([]string{"evergreen-ci"})
	assert.NotNil(t, err)

	// a project with an owner and repo should not error
	project.Owner = "evergreen-ci"
	err = project.ValidateOwnerAndRepo([]string{"evergreen-ci"})
	assert.NoError(t, err)
}

func TestProjectCanDispatchTask(t *testing.T) {
	t.Run("ReturnsTrueWithEnabledProject", func(t *testing.T) {
		pRef := ProjectRef{
			Enabled: true,
		}
		tsk := task.Task{
			Id: "id",
		}
		canDispatch, _ := ProjectCanDispatchTask(&pRef, &tsk)
		assert.True(t, canDispatch)
	})
	t.Run("ReturnsFalseWithDisabledProject", func(t *testing.T) {
		pRef := ProjectRef{
			Enabled: false,
		}
		tsk := task.Task{
			Id: "id",
		}
		canDispatch, reason := ProjectCanDispatchTask(&pRef, &tsk)
		assert.False(t, canDispatch)
		assert.NotZero(t, reason)
	})
	t.Run("ReturnsTrueWithDisabledHiddenProjectForGitHubPRTask", func(t *testing.T) {
		pRef := ProjectRef{
			Enabled: false,
			Hidden:  utility.TruePtr(),
		}
		tsk := task.Task{
			Id:        "id",
			Requester: evergreen.GithubPRRequester,
		}
		canDispatch, _ := ProjectCanDispatchTask(&pRef, &tsk)
		assert.True(t, canDispatch)
	})
	t.Run("ReturnsFalseWithDispatchingDisabledForPatchTask", func(t *testing.T) {
		pRef := ProjectRef{
			Enabled:             true,
			DispatchingDisabled: utility.TruePtr(),
		}
		tsk := task.Task{
			Id:        "id",
			Requester: evergreen.PatchVersionRequester,
		}
		canDispatch, reason := ProjectCanDispatchTask(&pRef, &tsk)
		assert.False(t, canDispatch)
		assert.NotZero(t, reason)
	})
	t.Run("ReturnsFalseWithDispatchingDisabledForMainlineTask", func(t *testing.T) {
		pRef := ProjectRef{
			Enabled:             true,
			DispatchingDisabled: utility.TruePtr(),
		}
		tsk := task.Task{
			Id:        "id",
			Requester: evergreen.RepotrackerVersionRequester,
		}
		canDispatch, reason := ProjectCanDispatchTask(&pRef, &tsk)
		assert.False(t, canDispatch)
		assert.NotZero(t, reason)
	})
	t.Run("ReturnsTrueWithPatchingDisabledForMainlineTask", func(t *testing.T) {
		pRef := ProjectRef{
			Enabled:          true,
			PatchingDisabled: utility.TruePtr(),
		}
		tsk := task.Task{
			Id:        "id",
			Requester: evergreen.RepotrackerVersionRequester,
		}
		canDispatch, _ := ProjectCanDispatchTask(&pRef, &tsk)
		assert.True(t, canDispatch)
	})
	t.Run("ReturnsFalseWithPatchingDisabledForPatchTask", func(t *testing.T) {
		pRef := ProjectRef{
			Enabled:          true,
			PatchingDisabled: utility.TruePtr(),
		}
		tsk := task.Task{
			Id:        "id",
			Requester: evergreen.PatchVersionRequester,
		}
		canDispatch, reason := ProjectCanDispatchTask(&pRef, &tsk)
		assert.False(t, canDispatch)
		assert.NotZero(t, reason)
	})
}

func TestGetNextCronTime(t *testing.T) {
	curTime := time.Date(2022, 12, 1, 0, 0, 0, 0, time.Local)
	cron := "0 * * * *"
	nextTime, err := GetNextCronTime(curTime, cron)
	assert.NoError(t, err)
	assert.NotEqual(t, nextTime, curTime)
	assert.Equal(t, nextTime, curTime.Add(time.Hour))

	// verify that a weekday cron can be parsed
	weekdayCron := "0 0 * * 1-5"
	_, err = GetNextCronTime(curTime, weekdayCron)
	assert.NoError(t, err)
}

func TestSetRepotrackerError(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(ProjectRefCollection))
	}()
	pRef := ProjectRef{
		Id:         "id",
		Identifier: "identifier",
		RepotrackerError: &RepositoryErrorDetails{
			InvalidRevision:   "abc123",
			MergeBaseRevision: "def456",
		},
	}
	require.NoError(t, pRef.Insert())
	t.Run("OverwritesError", func(t *testing.T) {
		repotrackerErr := &RepositoryErrorDetails{
			Exists:            true,
			InvalidRevision:   "invalid_revision",
			MergeBaseRevision: "merge_base_revision",
		}
		require.NoError(t, pRef.SetRepotrackerError(repotrackerErr))
		dbProjRef, err := FindBranchProjectRef(pRef.Identifier)
		require.NoError(t, err)
		require.NotZero(t, dbProjRef)
		require.NotZero(t, dbProjRef.RepotrackerError)
		assert.Equal(t, *repotrackerErr, *dbProjRef.RepotrackerError)
	})
	t.Run("ClearsError", func(t *testing.T) {
		require.NoError(t, pRef.SetRepotrackerError(&RepositoryErrorDetails{}))
		dbProjRef, err := FindBranchProjectRef(pRef.Identifier)
		require.NoError(t, err)
		require.NotZero(t, dbProjRef)
		assert.Empty(t, dbProjRef.RepotrackerError)
	})
}

func TestSetContainerSecrets(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(ProjectRefCollection))
	}()
	pRef := ProjectRef{
		Id:               "id",
		Identifier:       "identifier",
		ContainerSecrets: []ContainerSecret{{Name: "secret"}},
	}
	require.NoError(t, pRef.Insert())
	t.Run("OverwritesContainerSecrets", func(t *testing.T) {
		secrets := []ContainerSecret{{
			Name:         "new_secret",
			Type:         ContainerSecretPodSecret,
			ExternalName: "external_name",
			ExternalID:   "external_id",
		}}
		require.NoError(t, pRef.SetContainerSecrets(secrets))
		dbProjRef, err := FindBranchProjectRef(pRef.Identifier)
		require.NoError(t, err)
		require.NotZero(t, dbProjRef)
		require.NotZero(t, dbProjRef.ContainerSecrets)
		assert.Equal(t, secrets, dbProjRef.ContainerSecrets)
	})
	t.Run("ClearsContainerSecrets", func(t *testing.T) {
		require.NoError(t, pRef.SetContainerSecrets(nil))
		dbProjRef, err := FindBranchProjectRef(pRef.Identifier)
		require.NoError(t, err)
		require.NotZero(t, dbProjRef)
		assert.Empty(t, dbProjRef.RepotrackerError)
	})
}

func TestGetActivationTimeForVariant(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection))
	projectRef := &ProjectRef{
		Owner:      "mongodb",
		Repo:       "mci",
		Branch:     "main",
		Enabled:    true,
		Id:         "ident",
		Identifier: "identifier",
	}
	assert.Nil(projectRef.Insert())

	// Set based on last activation time when no version is found
	versionCreatedAt := time.Now().Add(-1 * time.Minute)
	activationTime, err := projectRef.GetActivationTimeForVariant(&BuildVariant{Name: "bv"}, versionCreatedAt, time.Now())
	assert.NoError(err)
	assert.Equal(activationTime, versionCreatedAt)

	// set based on last activation time with a version
	version := &Version{
		Id:                  "v1",
		Identifier:          "ident",
		RevisionOrderNumber: 10,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "bv",
				BuildId:      "build",
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
			},
		},
	}
	assert.Nil(version.Insert())

	activationTime, err = projectRef.GetActivationTimeForVariant(&BuildVariant{Name: "bv"}, versionCreatedAt, time.Now())
	assert.NoError(err)
	assert.NotZero(activationTime)
	assert.Equal(activationTime, versionCreatedAt)
}
