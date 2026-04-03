package model

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
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
	assert.NoError(projectRef.Insert(t.Context()))

	projectRefFromDB, err := FindBranchProjectRef(t.Context(), "ident")
	assert.NoError(err)
	assert.NotNil(projectRefFromDB)

	assert.Equal("mongodb", projectRefFromDB.Owner)
	assert.Equal("mci", projectRefFromDB.Repo)
	assert.Equal("main", projectRefFromDB.Branch)
	assert.True(projectRefFromDB.Enabled)
	assert.Equal(10, projectRefFromDB.BatchTime)
	assert.Equal("ident", projectRefFromDB.Id)
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
	assert.NoError(t, projectConfig.Insert(t.Context()))

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
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "project-filter",
				CaseSensitive: true,
				ExactMatch:    false,
			},
		},
	}
	assert.NoError(t, projectRef.Insert(t.Context()))
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
	assert.NoError(t, repoRef.Replace(t.Context()))

	mergedProject, err := FindMergedProjectRef(t.Context(), "ident", "ident", true)
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
	assert.Empty(t, mergedProject.GitTagAuthorizedTeams) // empty lists take precedent
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

	assert.NoError(t, projectRef.Replace(t.Context()))
	mergedProject, err = FindMergedProjectRef(t.Context(), "ident", "ident", true)
	assert.NoError(t, err)
	assert.Len(t, mergedProject.ParsleyFilters, 1)

	projectRef.ParsleyFilters = nil
	assert.NoError(t, projectRef.Replace(t.Context()))
	mergedProject, err = FindMergedProjectRef(t.Context(), "ident", "ident", true)
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
	assert.NoError(t, projectConfig.Insert(t.Context()))

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
	assert.NoError(t, projectConfig.Insert(t.Context()))
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
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "project-filter",
				CaseSensitive: true,
				ExactMatch:    false,
			},
		},
	}
	assert.NoError(t, projectRef.Insert(t.Context()))

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
		ParsleyFilters: []parsley.Filter{
			{
				Expression:    "project-filter",
				CaseSensitive: true,
				ExactMatch:    false,
			},
		},
	}
	assert.NoError(t, projectRef.Insert(t.Context()))

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
	assert.NoError(t, repoRef.Replace(t.Context()))

	mergedProjects, err := FindMergedEnabledProjectRefsByIds(t.Context(), "ident", "ident_enabled")
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
	assert.NoError(t, enabled1.Insert(t.Context()))
	enabled2 := &ProjectRef{
		Id:      "enabled2",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: true,
	}
	assert.NoError(t, enabled2.Insert(t.Context()))
	disabled1 := &ProjectRef{
		Id:      "disabled1",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: false,
	}
	assert.NoError(t, disabled1.Insert(t.Context()))
	disabled2 := &ProjectRef{
		Id:      "disabled2",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: false,
	}
	assert.NoError(t, disabled2.Insert(t.Context()))

	enabledProjects, err := GetNumberOfEnabledProjects(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 2, enabledProjects)
	enabledProjectsOwnerRepo, err := GetNumberOfEnabledProjectsForOwnerRepo(t.Context(), enabled2.Owner, enabled2.Repo)
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
	assert.NoError(t, enabled1.Insert(t.Context()))
	enabled2 := &ProjectRef{
		Id:      "enabled2",
		Owner:   "owner_exception",
		Repo:    "repo_exception",
		Enabled: true,
	}
	assert.NoError(t, enabled2.Insert(t.Context()))
	disabled1 := &ProjectRef{
		Id:      "disabled1",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: false,
	}
	assert.NoError(t, disabled1.Insert(t.Context()))
	enabledByRepo := &ProjectRef{
		Id:        "enabledByRepo",
		Owner:     "enable_mongodb",
		Repo:      "enable_mci",
		RepoRefId: "enable_repo",
	}
	assert.NoError(t, enabledByRepo.Insert(t.Context()))
	enableRef := &RepoRef{ProjectRef{
		Id:      "enable_repo",
		Owner:   "enable_mongodb",
		Repo:    "enable_mci",
		Enabled: true,
	}}
	assert.NoError(t, enableRef.Replace(t.Context()))
	disabledByRepo := &ProjectRef{
		Id:        "disabledByRepo",
		Owner:     "disable_mongodb",
		Repo:      "disable_mci",
		RepoRefId: "disable_repo",
	}
	assert.NoError(t, disabledByRepo.Insert(t.Context()))
	disableRepo := &RepoRef{ProjectRef{
		Id:      "disable_repo",
		Owner:   "disable_mongodb",
		Repo:    "disable_mci",
		Enabled: true,
	}}
	assert.NoError(t, disableRepo.Replace(t.Context()))

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
	original, err := FindMergedProjectRef(t.Context(), disabled1.Id, "", false)
	assert.NoError(t, err)
	statusCode, err := ValidateEnabledProjectsLimit(t.Context(), &settings, original, disabled1)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)

	// Should not error if owner/repo is part of exception.
	exception := &ProjectRef{
		Id:      "exception",
		Owner:   "owner_exception",
		Repo:    "repo_exception",
		Enabled: true,
	}
	original, err = FindMergedProjectRef(t.Context(), exception.Id, "", false)
	assert.NoError(t, err)
	_, err = ValidateEnabledProjectsLimit(t.Context(), &settings, original, exception)
	assert.NoError(t, err)

	// Should error if owner/repo is not part of exception.
	notException := &ProjectRef{
		Id:      "not_exception",
		Owner:   "mongodb",
		Repo:    "mci",
		Enabled: true,
	}
	original, err = FindMergedProjectRef(t.Context(), notException.Id, "", false)
	assert.NoError(t, err)
	statusCode, err = ValidateEnabledProjectsLimit(t.Context(), &settings, original, notException)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)

	// Should not error if a repo defaulted project is enabled.
	disableRepo.Enabled = true
	assert.NoError(t, disableRepo.Replace(t.Context()))
	mergedRef, err := GetProjectRefMergedWithRepo(t.Context(), *disabledByRepo)
	assert.NoError(t, err)
	original, err = FindMergedProjectRef(t.Context(), disabledByRepo.Id, "", false)
	assert.NoError(t, err)
	_, err = ValidateEnabledProjectsLimit(t.Context(), &settings, original, mergedRef)
	assert.NoError(t, err)

	// Should error on enabled if you try to change owner/repo past limit.
	enabled2.Owner = "mongodb"
	enabled2.Repo = "mci"
	original, err = FindMergedProjectRef(t.Context(), enabled2.Id, "", false)
	assert.NoError(t, err)
	statusCode, err = ValidateEnabledProjectsLimit(t.Context(), &settings, original, enabled2)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)

	// Total project limit cannot be exceeded. Even with the exception.
	settings.ProjectCreation.TotalProjectLimit = 2
	original, err = FindMergedProjectRef(t.Context(), exception.Id, "", false)
	assert.NoError(t, err)
	statusCode, err = ValidateEnabledProjectsLimit(t.Context(), &settings, original, exception)
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

	assert.Equal(maxBatchTime, projectRef.getBatchTimeForVariant(emptyVariant),
		"ProjectRef.getBatchTimeForVariant() is not capping BatchTime to MaxInt32")

	assert.Equal(maxBatchTime, projectRef.getBatchTimeForTask(emptyTask),
		"ProjectRef.getBatchTimeForTask() is not capping BatchTime to MaxInt32")

	projectRef.BatchTime = 55
	assert.Equal(55, projectRef.getBatchTimeForVariant(emptyVariant),
		"ProjectRef.getBatchTimeForVariant() is not returning the correct BatchTime")

	assert.Equal(55, projectRef.getBatchTimeForTask(emptyTask),
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
	assert.NoError(t, versionWithoutTask.Insert(t.Context()))
	assert.NoError(t, versionWithTask.Insert(t.Context()))

	currentTime := time.Now()
	activationTime, err := projectRef.GetActivationTimeForTask(t.Context(), bvt, currentTime, time.Now())
	assert.NoError(t, err)
	assert.True(t, activationTime.Equal(prevTime.Add(time.Hour)))

	// Activation time should be the zero time, because this variant is disabled.
	activationTime, err = projectRef.GetActivationTimeForTask(t.Context(), bvt2, currentTime, time.Now())
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
			return pRef.GetActivationTimeForTask(t.Context(), &bvtu, versionCreatedAt, now)
		},
		"Variant": func(versionCreatedAt time.Time, now time.Time) (time.Time, error) {
			return pRef.GetActivationTimeForVariant(t.Context(), &bv, false, versionCreatedAt, now)
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
					require.NoError(t, conflictingVersionWithCron.Insert(t.Context()))

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
					require.NoError(t, recentVersionWithCron.Insert(t.Context()))

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
					require.NoError(t, recentVersionWithCron.Insert(t.Context()))

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
					require.NoError(t, conflictingVersionWithCron.Insert(t.Context()))

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
	assert.NoError(t, pRef.Insert(t.Context()))
	repoRef := RepoRef{ProjectRef{
		Id: "myRepo",
	}}
	assert.NoError(t, repoRef.Replace(t.Context()))
	u := &user.DBUser{Id: "me"}

	assert.NoError(t, u.Insert(t.Context()))
	installation := githubapp.GitHubAppInstallation{
		Owner:          pRef.Owner,
		Repo:           pRef.Repo,
		AppID:          1234,
		InstallationID: 5678,
	}
	assert.NoError(t, installation.Upsert(ctx))

	// Can't attach to repo with an invalid owner
	pRef.Owner = "invalid"
	assert.Error(t, pRef.AttachToNewRepo(t.Context(), u))

	pRef.Owner = "newOwner"
	pRef.Repo = "newRepo"
	newInstallation := githubapp.GitHubAppInstallation{
		Owner:          pRef.Owner,
		Repo:           pRef.Repo,
		AppID:          1234,
		InstallationID: 5678,
	}
	assert.NoError(t, newInstallation.Upsert(ctx))
	assert.NoError(t, pRef.AttachToNewRepo(t.Context(), u))

	pRefFromDB, err := FindBranchProjectRef(t.Context(), pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotEqual(t, "myRepo", pRefFromDB.RepoRefId)
	assert.Equal(t, "newOwner", pRefFromDB.Owner)
	assert.Equal(t, "newRepo", pRefFromDB.Repo)
	assert.Nil(t, pRefFromDB.TracksPushEvents)

	newRepoRef, err := FindOneRepoRef(t.Context(), pRef.RepoRefId)
	assert.NoError(t, err)
	assert.NotNil(t, newRepoRef)

	assert.True(t, newRepoRef.DoesTrackPushEvents())

	mergedRef, err := FindMergedProjectRef(t.Context(), pRef.Id, "", false)
	assert.NoError(t, err)
	assert.True(t, mergedRef.DoesTrackPushEvents())

	userFromDB, err := user.FindOneById(t.Context(), "me")
	assert.NoError(t, err)
	assert.Len(t, userFromDB.SystemRoles, 1)
	assert.Contains(t, userFromDB.SystemRoles, GetRepoAdminRole(pRefFromDB.RepoRefId))
	hasPermission, err := UserHasRepoViewPermission(t.Context(), u, pRefFromDB.RepoRefId)
	assert.NoError(t, err)
	assert.True(t, hasPermission)
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
	assert.NoError(t, pRef.Insert(t.Context()))
	pRef.Owner = "newOwner"
	pRef.Repo = "newRepo"
	assert.NoError(t, pRef.AttachToNewRepo(t.Context(), u))
	assert.True(t, pRef.UseRepoSettings())
	assert.NotEmpty(t, pRef.RepoRefId)

	pRefFromDB, err = FindBranchProjectRef(t.Context(), pRef.Id)
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
	events, err := MostRecentProjectEvents(t.Context(), project.Id, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, project.Id, events[0].ResourceId)
	assert.Equal(t, event.EventResourceTypeProject, events[0].ResourceType)
	assert.Equal(t, attachmentType, events[0].EventType)
}

func TestAttachToRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, evergreen.ScopeCollection,
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
	assert.NoError(t, pRef.Insert(t.Context()))

	installation := githubapp.GitHubAppInstallation{
		Owner:          pRef.Owner,
		Repo:           pRef.Repo,
		AppID:          1234,
		InstallationID: 5678,
	}
	assert.NoError(t, installation.Upsert(ctx))

	u := &user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert(t.Context()))
	// No repo exists, but one should be created.
	assert.NoError(t, pRef.AttachToRepo(ctx, u))
	assert.True(t, pRef.UseRepoSettings())
	assert.NotEmpty(t, pRef.RepoRefId)
	checkRepoAttachmentEventLog(t, pRef, event.EventTypeProjectAttachedToRepo)

	pRefFromDB, err := FindBranchProjectRef(t.Context(), pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.True(t, pRefFromDB.UseRepoSettings())
	assert.NotEmpty(t, pRefFromDB.RepoRefId)
	assert.True(t, pRefFromDB.Enabled)
	assert.True(t, pRefFromDB.CommitQueue.IsEnabled())
	assert.True(t, pRefFromDB.IsGithubChecksEnabled())
	assert.Nil(t, pRefFromDB.TracksPushEvents)

	repoRef, err := FindOneRepoRef(t.Context(), pRef.RepoRefId)
	assert.NoError(t, err)
	require.NotNil(t, repoRef)
	assert.True(t, repoRef.DoesTrackPushEvents())

	u, err = user.FindOneById(t.Context(), "me")
	assert.NoError(t, err)
	assert.NotNil(t, u)
	assert.Contains(t, u.Roles(), GetRepoAdminRole(pRefFromDB.RepoRefId))
	hasPermission, err := UserHasRepoViewPermission(t.Context(), u, pRefFromDB.RepoRefId)
	assert.NoError(t, err)
	assert.True(t, hasPermission)

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
	assert.NoError(t, pRef.Insert(t.Context()))
	assert.NoError(t, pRef.AttachToRepo(ctx, u))
	assert.True(t, pRef.UseRepoSettings())
	assert.NotEmpty(t, pRef.RepoRefId)
	checkRepoAttachmentEventLog(t, pRef, event.EventTypeProjectAttachedToRepo)

	pRefFromDB, err = FindBranchProjectRef(t.Context(), pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.True(t, pRefFromDB.UseRepoSettings())
	assert.NotEmpty(t, pRefFromDB.RepoRefId)
	// Commit queue and github checks should be set to false, since they would introduce project conflicts.
	assert.False(t, pRefFromDB.CommitQueue.IsEnabled())
	assert.False(t, pRefFromDB.IsGithubChecksEnabled())
	assert.True(t, pRefFromDB.IsPRTestingEnabled())

	repoVars, err := FindOneProjectVars(t.Context(), pRef.RepoRefId)
	assert.NoError(t, err)
	assert.NotNil(t, repoVars)

	// Attaching a new project should recreate the vars if they don't exist.
	require.NoError(t, db.ClearCollections(ProjectVarsCollection))
	repoVars, err = FindOneProjectVars(t.Context(), pRef.RepoRefId)
	assert.NoError(t, err)
	assert.Nil(t, repoVars)

	pRef = ProjectRef{
		Id:      "myProjectMissingVars",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		Admins:  []string{"me"},
		Enabled: true,
	}
	assert.NoError(t, pRef.Insert(t.Context()))
	assert.NoError(t, pRef.AttachToRepo(ctx, u))
	assert.True(t, pRef.UseRepoSettings())

	repoVars, err = FindOneProjectVars(t.Context(), pRef.RepoRefId)
	assert.NoError(t, err)
	assert.NotNil(t, repoVars, "project vars should be recreated for repo ref missing them")

	// Try attaching with a disallowed owner.
	pRef = ProjectRef{
		Id:     "myBadProject",
		Owner:  "nonexistent",
		Repo:   "evergreen",
		Branch: "main",
	}
	assert.NoError(t, pRef.Insert(t.Context()))
	assert.Error(t, pRef.AttachToRepo(ctx, u))

	// Try attaching with project admin but not repo admin.
	pRef = ProjectRef{
		Id:      "myThirdProject",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		Admins:  []string{"nonRepoAdmin"},
		Enabled: true,
	}
	assert.NoError(t, pRef.Insert(t.Context()))

	nonRepoAdmin := &user.DBUser{
		Id:          "nonRepoAdmin",
		SystemRoles: []string{GetProjectAdminRole(pRef.Id)},
	}

	hasRepoPermission, err := UserHasRepoViewPermission(t.Context(), nonRepoAdmin, pRef.RepoRefId)
	assert.NoError(t, err)
	assert.False(t, hasRepoPermission)

	assert.Error(t, pRef.AttachToRepo(ctx, nonRepoAdmin))
}

func checkParametersMatchVars(ctx context.Context, t *testing.T, pm ParameterMappings, vars map[string]string) {
	assert.Len(t, pm, len(vars), "each project var should have exactly one corresponding parameter")
	fakeParams, err := fakeparameter.FindByIDs(ctx, pm.ParameterNames()...)
	assert.NoError(t, err)
	assert.Len(t, fakeParams, len(vars), "number of parameters for project vars should match number of project vars defined")

	paramNamesMap := pm.ParameterNameMap()
	for _, fakeParam := range fakeParams {
		varName := paramNamesMap[fakeParam.Name].Name
		assert.NotEmpty(t, varName, "parameter should have corresponding project variable")
		assert.Equal(t, vars[varName], fakeParam.Value, "project variable value should match the parameter value")
	}
}

// checkParametersNamespacedByProject checks that the parameter names for the
// project vars all include the project ID as a prefix.
func checkParametersNamespacedByProject(t *testing.T, vars ProjectVars) {
	projectID := vars.Id

	commonAndProjectIDPrefix := fmt.Sprintf("/%s/%s/", strings.TrimSuffix(strings.TrimPrefix(evergreen.GetEnvironment().Settings().ParameterStore.Prefix, "/"), "/"), GetVarsParameterPath(projectID))
	for _, pm := range vars.Parameters {
		assert.True(t, strings.HasPrefix(pm.ParameterName, commonAndProjectIDPrefix), "parameter name '%s' should have standard prefix '%s'", pm.ParameterName, commonAndProjectIDPrefix)
	}
}

func TestDetachFromRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for name, test := range map[string]func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser){
		"ProjectRefIsUpdatedCorrectly": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			pRefFromDB, err := FindBranchProjectRef(t.Context(), pRef.Id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDB)
			assert.False(t, pRefFromDB.UseRepoSettings())
			assert.Empty(t, pRefFromDB.RepoRefId)
			assert.NotNil(t, pRefFromDB.PRTestingEnabled)
			assert.False(t, pRefFromDB.IsPRTestingEnabled())
			assert.NotNil(t, pRefFromDB.GitTagVersionsEnabled)
			assert.True(t, pRefFromDB.IsGitTagVersionsEnabled())
			assert.True(t, pRefFromDB.IsGithubChecksEnabled())
			assert.Equal(t, []string{"my_trigger"}, pRefFromDB.GithubPRTriggerAliases)
			assert.True(t, pRefFromDB.DoesTrackPushEvents())

			dbUser, err = user.FindOneById(t.Context(), "me")
			assert.NoError(t, err)
			assert.NotNil(t, dbUser)
			hasPermission, err := UserHasRepoViewPermission(t.Context(), dbUser, pRefFromDB.RepoRefId)
			assert.NoError(t, err)
			assert.False(t, hasPermission)
		},
		"NewRepoVarsAreMerged": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			vars, err := FindOneProjectVars(t.Context(), pRef.Id)
			assert.NoError(t, err)
			assert.NotNil(t, vars)
			assert.Equal(t, "only", vars.Vars["project"])
			assert.Equal(t, "both", vars.Vars["in"])    // not modified
			assert.Equal(t, "only!", vars.Vars["repo"]) // added from repo
			assert.False(t, vars.PrivateVars["project"])
			assert.True(t, vars.PrivateVars["in"])
			assert.True(t, vars.PrivateVars["repo"]) // added from repo
		},
		"ProjectAndRepoVarsAreMergedAndStoredInParameterStore": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			expectedVars, err := FindMergedProjectVars(t.Context(), pRef.Id)
			require.NoError(t, err)

			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			vars, err := FindOneProjectVars(t.Context(), pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, vars)

			checkParametersMatchVars(ctx, t, vars.Parameters, expectedVars.Vars)
			checkParametersNamespacedByProject(t, *vars)
		},
		"PatchAliases": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			// no patch aliases are copied if the project has a patch alias
			projectAlias := ProjectAlias{Alias: "myProjectAlias", ProjectID: pRef.Id}
			assert.NoError(t, projectAlias.Upsert(t.Context()))

			repoAlias := ProjectAlias{Alias: "myRepoAlias", ProjectID: pRef.RepoRefId}
			assert.NoError(t, repoAlias.Upsert(t.Context()))

			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			aliases, err := FindAliasesForProjectFromDb(t.Context(), pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			assert.Equal(t, aliases[0].Alias, projectAlias.Alias)

			// reattach to repo to test without project patch aliases
			assert.NoError(t, pRef.AttachToRepo(ctx, dbUser))
			assert.NotEmpty(t, pRef.RepoRefId)
			assert.True(t, pRef.UseRepoSettings())
			assert.NoError(t, RemoveProjectAlias(ctx, projectAlias.ID.Hex()))

			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))
			aliases, err = FindAliasesForProjectFromDb(t.Context(), pRef.Id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			assert.Equal(t, aliases[0].Alias, repoAlias.Alias)
		},
		"InternalAliases": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
			projectAliases := []ProjectAlias{
				{Alias: evergreen.GitTagAlias, Variant: "projectVariant"},
				{Alias: evergreen.CommitQueueAlias},
			}
			assert.NoError(t, UpsertAliasesForProject(t.Context(), projectAliases, pRef.Id))
			repoAliases := []ProjectAlias{
				{Alias: evergreen.GitTagAlias, Variant: "repoVariant"},
				{Alias: evergreen.GithubPRAlias},
			}
			assert.NoError(t, UpsertAliasesForProject(t.Context(), repoAliases, pRef.RepoRefId))

			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)
			aliases, err := FindAliasesForProjectFromDb(t.Context(), pRef.Id)
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
			assert.Equal(t, 1, gitTagCount)
			assert.Equal(t, 1, prCount)
			assert.Equal(t, 1, cqCount)
		},
		"Subscriptions": func(t *testing.T, pRef *ProjectRef, dbUser *user.DBUser) {
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
			assert.NoError(t, projectSubscription.Upsert(t.Context()))
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
			assert.NoError(t, repoSubscription.Upsert(t.Context()))
			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))
			checkRepoAttachmentEventLog(t, *pRef, event.EventTypeProjectDetachedFromRepo)

			subs, err := event.FindSubscriptionsByOwner(t.Context(), pRef.Id, event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, subs, 1)
			assert.Equal(t, subs[0].Owner, pRef.Id)
			assert.Equal(t, event.TriggerOutcome, subs[0].Trigger)

			// reattach to repo to test without subscription
			assert.NoError(t, pRef.AttachToRepo(ctx, dbUser))
			assert.NoError(t, event.RemoveSubscription(ctx, projectSubscription.ID))
			assert.NoError(t, pRef.DetachFromRepo(t.Context(), dbUser))

			subs, err = event.FindSubscriptionsByOwner(t.Context(), pRef.Id, event.OwnerTypeProject)
			assert.NoError(t, err)
			assert.Len(t, subs, 1)
			assert.Equal(t, subs[0].Owner, pRef.Id)
			assert.Equal(t, event.TriggerFailure, subs[0].Trigger)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection,
				evergreen.RoleCollection, user.Collection, event.SubscriptionsCollection, event.EventCollection, ProjectAliasCollection, ProjectVarsCollection, fakeparameter.Collection))
			require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

			pRef := &ProjectRef{
				Id:                    "myProject",
				Owner:                 "evergreen-ci",
				Repo:                  "evergreen",
				RepoRefId:             "myRepo",
				PeriodicBuilds:        []PeriodicBuildDefinition{}, // also shouldn't be overwritten
				PRTestingEnabled:      utility.FalsePtr(),          // neither of these should be changed when overwriting
				GitTagVersionsEnabled: utility.TruePtr(),
				GithubChecksEnabled:   nil, // for now this is defaulting to repo
			}
			assert.NoError(t, pRef.Insert(t.Context()))

			repoRef := RepoRef{ProjectRef{
				Id:                     pRef.RepoRefId,
				Owner:                  pRef.Owner,
				Repo:                   pRef.Repo,
				TracksPushEvents:       utility.TruePtr(),
				PRTestingEnabled:       utility.TruePtr(),
				GitTagVersionsEnabled:  utility.FalsePtr(),
				GithubChecksEnabled:    utility.TruePtr(),
				GithubPRTriggerAliases: []string{"my_trigger"},
				Admins:                 []string{"me"},
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "my_build"},
				},
			}}
			assert.NoError(t, repoRef.Replace(t.Context()))

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
			_, err := pVars.Upsert(t.Context())
			assert.NoError(t, err)

			dbProjRef, err := FindBranchProjectRef(t.Context(), pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)

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
			_, err = repoVars.Upsert(t.Context())
			assert.NoError(t, err)

			dbRepoRef, err := FindOneRepoRef(t.Context(), repoRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbRepoRef)

			u := &user.DBUser{
				Id: "me",
			}
			assert.NoError(t, u.Insert(t.Context()))
			assert.NoError(t, repoRef.addPermissions(t.Context(), u))

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
			assert.NoError(t, repoRef.Replace(t.Context()))
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageGeneralSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.NotEqual(t, "", pRefFromDb.Identifier)
			assert.Equal(t, 0, pRefFromDb.BatchTime)
			assert.Nil(t, pRefFromDb.RepotrackerDisabled)
			assert.Nil(t, pRefFromDb.DeactivatePrevious)
			assert.Empty(t, pRefFromDb.RemotePath)
		},
		ProjectPageAccessSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageAccessSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.Restricted)
			assert.Nil(t, pRefFromDb.Admins)
		},
		ProjectPageVariablesSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageVariablesSection, "me"))

			varsFromDb, err := FindOneProjectVars(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, varsFromDb)
			assert.Empty(t, varsFromDb.Vars)
			assert.Empty(t, varsFromDb.PrivateVars)
			assert.NotEmpty(t, varsFromDb.Id)
		},
		ProjectPageGithubAndCQSection: func(t *testing.T, id string) {
			aliases, err := FindAliasesForProjectFromDb(t.Context(), id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 5)
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageGithubAndCQSection, "me"))

			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PRTestingEnabled)
			assert.Nil(t, pRefFromDb.GithubChecksEnabled)
			assert.Nil(t, pRefFromDb.GitTagAuthorizedUsers)
			aliases, err = FindAliasesForProjectFromDb(t.Context(), id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 1)
			// assert that only patch aliases are left
			for _, a := range aliases {
				assert.NotContains(t, evergreen.InternalAliases, a.Alias)
			}
		},
		ProjectPageNotificationsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageNotificationsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.NotifyOnBuildFailure)
		},
		ProjectPagePatchAliasSection: func(t *testing.T, id string) {
			aliases, err := FindAliasesForProjectFromDb(t.Context(), id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 5)

			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPagePatchAliasSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PatchTriggerAliases)

			aliases, err = FindAliasesForProjectFromDb(t.Context(), id)
			assert.NoError(t, err)
			assert.Len(t, aliases, 4)
			// assert that no patch aliases are left
			for _, a := range aliases {
				assert.Contains(t, evergreen.InternalAliases, a.Alias)
			}
		},
		ProjectPageTriggersSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageTriggersSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.Triggers)
		},
		ProjectPageWorkstationsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageWorkstationsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.WorkstationConfig.GitClone)
			assert.Nil(t, pRefFromDb.WorkstationConfig.SetupCommands)
		},
		ProjectPagePluginSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPagePluginSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Equal(t, "", pRefFromDb.TaskAnnotationSettings.FileTicketWebhook.Endpoint)
			assert.Equal(t, "", pRefFromDb.BuildBaronSettings.TicketCreateProject)
			assert.Nil(t, pRefFromDb.PerfEnabled)
		},
		ProjectPagePeriodicBuildsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPagePeriodicBuildsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.PeriodicBuilds)
		},
		ProjectPageGithubAppSettingsSection: func(t *testing.T, id string) {
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)

			auth := githubapp.GithubAppAuth{
				Id:         pRefFromDb.RepoRefId,
				AppID:      9999,
				PrivateKey: []byte("repo-secret"),
			}
			err = githubapp.UpsertGitHubAppAuth(t.Context(), &auth)
			assert.NoError(t, err)
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageGithubAppSettingsSection, "me"))
			pRefFromDb, err = FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			require.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.GitHubDynamicTokenPermissionGroups)
			assert.Nil(t, pRefFromDb.GitHubPermissionGroupByRequester)

			app, err := pRefFromDb.GetGitHubAppAuth(t.Context())
			require.NoError(t, err)
			require.NotNil(t, app)
			assert.Equal(t, pRefFromDb.RepoRefId, app.Id)
		},
		ProjectPageGithubPermissionsSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageGithubPermissionsSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.GitHubDynamicTokenPermissionGroups)
		},
		ProjectPageTestSelectionSection: func(t *testing.T, id string) {
			assert.NoError(t, DefaultSectionToRepo(t.Context(), id, ProjectPageTestSelectionSection, "me"))
			pRefFromDb, err := FindBranchProjectRef(t.Context(), id)
			assert.NoError(t, err)
			assert.NotNil(t, pRefFromDb)
			assert.Nil(t, pRefFromDb.TestSelection.Allowed)
			assert.Nil(t, pRefFromDb.TestSelection.DefaultEnabled)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(ProjectRefCollection, ProjectVarsCollection, fakeparameter.Collection, ProjectAliasCollection,
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
			assert.NoError(t, pRef.Insert(t.Context()))

			repoRef := RepoRef{
				ProjectRef: ProjectRef{
					Id: pRef.RepoRefId,
				},
			}
			assert.NoError(t, repoRef.Replace(t.Context()))

			pVars := ProjectVars{
				Id:          pRef.Id,
				Vars:        map[string]string{"hello": "world"},
				PrivateVars: map[string]bool{"hello": true},
			}
			assert.NoError(t, pVars.Insert(t.Context()))
			checkParametersNamespacedByProject(t, pVars)

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
				assert.NoError(t, a.Upsert(t.Context()))
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
	require.NoError(p1.Insert(t.Context()))
	p2 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "not-mci1",
		Branch:  "main",
		Id:      "p2",
		Enabled: true,
	}
	require.NoError(p2.Insert(t.Context()))
	conflicts, err := p1.GetGithubProjectConflicts(t.Context())
	require.NoError(err)
	assert.Empty(conflicts.PRTestingIdentifiers)
	assert.Empty(conflicts.CommitQueueIdentifiers)
	assert.Empty(conflicts.CommitCheckIdentifiers)

	// Two project refs that are from the same repo but do not have potential conflicting settings.
	p3 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci2",
		Branch:  "main",
		Id:      "p3",
		Enabled: true,
	}
	require.NoError(p3.Insert(t.Context()))
	p4 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci2",
		Branch:  "main",
		Id:      "p4",
		Enabled: true,
	}
	require.NoError(p4.Insert(t.Context()))
	conflicts, err = p3.GetGithubProjectConflicts(t.Context())
	require.NoError(err)
	assert.Empty(conflicts.PRTestingIdentifiers)
	assert.Empty(conflicts.CommitQueueIdentifiers)
	assert.Empty(conflicts.CommitCheckIdentifiers)

	// Three project refs that do have potential conflicting settings.
	p5 := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci3",
		Branch:           "main",
		Id:               "p5",
		Enabled:          true,
		PRTestingEnabled: utility.TruePtr(),
	}
	require.NoError(p5.Insert(t.Context()))
	p6 := &ProjectRef{
		Owner:       "mongodb",
		Repo:        "mci3",
		Branch:      "main",
		Id:          "p6",
		Enabled:     true,
		CommitQueue: CommitQueueParams{Enabled: utility.TruePtr()},
	}
	require.NoError(p6.Insert(t.Context()))
	p7 := &ProjectRef{
		Owner:               "mongodb",
		Repo:                "mci3",
		Branch:              "main",
		Id:                  "p7",
		Enabled:             true,
		GithubChecksEnabled: utility.TruePtr(),
	}
	require.NoError(p7.Insert(t.Context()))
	p8 := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci3",
		Branch:  "main",
		Id:      "p8",
		Enabled: true,
	}
	require.NoError(p8.Insert(t.Context()))
	// p5 should have conflicting with commit queue and commit check.
	conflicts, err = p5.GetGithubProjectConflicts(t.Context())
	require.NoError(err)
	assert.Empty(conflicts.PRTestingIdentifiers)
	assert.Len(conflicts.CommitQueueIdentifiers, 1)
	assert.Len(conflicts.CommitCheckIdentifiers, 1)
	// p6 should have conflicting with pr testing and commit check.
	conflicts, err = p6.GetGithubProjectConflicts(t.Context())
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 1)
	assert.Empty(conflicts.CommitQueueIdentifiers)
	assert.Len(conflicts.CommitCheckIdentifiers, 1)
	// p7 should have conflicting with pr testing and commit queue.
	conflicts, err = p7.GetGithubProjectConflicts(t.Context())
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 1)
	assert.Len(conflicts.CommitQueueIdentifiers, 1)
	assert.Empty(conflicts.CommitCheckIdentifiers)
	// p8 should have conflicting with all
	conflicts, err = p8.GetGithubProjectConflicts(t.Context())
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
	require.NoError(p9.Insert(t.Context()))
	r9 := &RepoRef{
		ProjectRef: *p9,
	}
	require.NoError(r9.Replace(t.Context()))
	p10 := &ProjectRef{
		Owner:     "mongodb",
		Repo:      "mci4",
		Branch:    "main",
		Id:        "p10",
		Enabled:   true,
		RepoRefId: p9.Id,
	}
	require.NoError(p10.Insert(t.Context()))
	// p9 should not have any potential conflicts.
	conflicts, err = p9.GetGithubProjectConflicts(t.Context())
	require.NoError(err)
	assert.Empty(conflicts.PRTestingIdentifiers)
	assert.Empty(conflicts.CommitQueueIdentifiers)
	assert.Empty(conflicts.CommitCheckIdentifiers)
	// p10 should have a potential conflict because p9 has something enabled.
	conflicts, err = p10.GetGithubProjectConflicts(t.Context())
	require.NoError(err)
	assert.Len(conflicts.PRTestingIdentifiers, 1)
	assert.Empty(conflicts.CommitQueueIdentifiers)
	assert.Empty(conflicts.CommitCheckIdentifiers)
}

func TestFindProjectRefsByRepoAndBranch(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	assert.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	projectRefs, err := FindMergedEnabledProjectRefsByRepoAndBranch(t.Context(), "mongodb", "mci", "main")
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
	assert.NoError(projectRef.Insert(t.Context()))
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch(t.Context(), "mongodb", "mci", "main")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef.Id = "ident"
	projectRef.Enabled = true
	assert.NoError(projectRef.Insert(t.Context()))

	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch(t.Context(), "mongodb", "mci", "main")
	assert.NoError(err)
	require.Len(projectRefs, 1)
	assert.Equal("ident", projectRefs[0].Id)

	projectRef.Id = "ident2"
	assert.NoError(projectRef.Insert(t.Context()))
	projectRefs, err = FindMergedEnabledProjectRefsByRepoAndBranch(t.Context(), "mongodb", "mci", "main")
	assert.NoError(err)
	assert.Len(projectRefs, 2)
}

func TestSetGithubAppCredentials(t *testing.T) {
	sampleAppId := int64(10)
	samplePrivateKey := []byte("private_key")
	for name, test := range map[string]func(t *testing.T, p *ProjectRef){
		"NoCredentialsWhenNoneExist": func(t *testing.T, p *ProjectRef) {
			app, err := githubapp.FindOneGitHubAppAuth(t.Context(), p.Id)
			require.NoError(t, err)
			assert.Nil(t, app)
		},
		"CredentialsCanBeSet": func(t *testing.T, p *ProjectRef) {
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), sampleAppId, samplePrivateKey))
			app, err := githubapp.FindOneGitHubAppAuth(t.Context(), p.Id)
			require.NoError(t, err)
			assert.Equal(t, sampleAppId, app.AppID)
			assert.Equal(t, samplePrivateKey, app.PrivateKey)
		},
		"CredentialsCanBeRemovedByEmptyAppIDAndEmptyPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), sampleAppId, samplePrivateKey))
			app, err := githubapp.FindOneGitHubAppAuth(t.Context(), p.Id)
			require.NoError(t, err)
			assert.Equal(t, sampleAppId, app.AppID)
			assert.Equal(t, samplePrivateKey, app.PrivateKey)

			// Remove credentials.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), 0, []byte("")))
			app, err = githubapp.FindOneGitHubAppAuth(t.Context(), p.Id)
			require.NoError(t, err)
			assert.Nil(t, app)

			// Attempting to remove credentials again should not error.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), 0, []byte("")))
			app, err = githubapp.FindOneGitHubAppAuth(t.Context(), p.Id)
			require.NoError(t, err)
			assert.Nil(t, app)
		},
		"CredentialsCanBeRemovedByEmptyAppIDAndNilPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), sampleAppId, samplePrivateKey))
			app, err := githubapp.FindOneGitHubAppAuth(t.Context(), p.Id)
			require.NoError(t, err)
			assert.Equal(t, sampleAppId, app.AppID)
			assert.Equal(t, samplePrivateKey, app.PrivateKey)

			// Remove credentials.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), 0, nil))
			app, err = githubapp.FindOneGitHubAppAuth(t.Context(), p.Id)
			require.NoError(t, err)
			assert.Nil(t, app)
		},
		"CredentialsCannotBeRemovedByOnlyEmptyPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), sampleAppId, samplePrivateKey))
			appID, err := githubapp.GetGitHubAppID(t.Context(), p.Id)
			require.NoError(t, err)
			assert.NotNil(t, appID)

			// Remove credentials.
			require.Error(t, p.SetGithubAppCredentials(t.Context(), sampleAppId, []byte("")), "both app ID and private key must be provided")
		},
		"CredentialsCannotBeRemovedByOnlyNilPrivateKey": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), 10, samplePrivateKey))
			appID, err := githubapp.GetGitHubAppID(t.Context(), p.Id)
			require.NoError(t, err)
			assert.NotNil(t, appID)

			// Remove credentials.
			require.Error(t, p.SetGithubAppCredentials(t.Context(), sampleAppId, nil), "both app ID and private key must be provided")
		},
		"CredentialsCannotBeRemovedByOnlyEmptyAppID": func(t *testing.T, p *ProjectRef) {
			// Add credentials.
			require.NoError(t, p.SetGithubAppCredentials(t.Context(), sampleAppId, samplePrivateKey))
			appID, err := githubapp.GetGitHubAppID(t.Context(), p.Id)
			require.NoError(t, err)
			assert.NotNil(t, appID)

			// Remove credentials.
			require.Error(t, p.SetGithubAppCredentials(t.Context(), 0, samplePrivateKey), "both app ID and private key must be provided")
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectRefCollection, githubapp.GitHubAppAuthCollection))
			p := &ProjectRef{
				Id: "id1",
			}
			require.NoError(t, p.Insert(t.Context()))
			test(t, p)
		})
	}
}

func TestCreateNewRepoRef(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, user.Collection,
		evergreen.ScopeCollection, ProjectVarsCollection, fakeparameter.Collection, ProjectAliasCollection, githubapp.GitHubAppCollection))
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
	}
	assert.NoError(t, doc1.Insert(t.Context()))
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
	}
	assert.NoError(t, doc2.Insert(t.Context()))
	doc3 := &ProjectRef{
		Id:      "id3",
		Owner:   "mongodb",
		Repo:    "mongo",
		Branch:  "mci2",
		Enabled: false,
	}
	assert.NoError(t, doc3.Insert(t.Context()))

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
				"this_is_only": "in one doc",
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
				"its_me": "adele",
			},
		},
	}
	for _, vars := range projectVariables {
		assert.NoError(t, vars.Insert(t.Context()))
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
		assert.NoError(t, a.Upsert(t.Context()))
	}
	u := user.DBUser{Id: "me"}
	assert.NoError(t, u.Insert(t.Context()))

	// This will create the new repo ref
	assert.NoError(t, doc2.AddToRepoScope(t.Context(), &u))
	assert.NotEmpty(t, doc2.RepoRefId)

	repoRef, err := FindOneRepoRef(t.Context(), doc2.RepoRefId)
	assert.NoError(t, err)
	assert.NotNil(t, repoRef)

	assert.Equal(t, "mongodb", repoRef.Owner)
	assert.Equal(t, "mongo", repoRef.Repo)
	assert.Empty(t, repoRef.Branch)
	assert.True(t, repoRef.DoesTrackPushEvents())
	assert.True(t, repoRef.IsPRTestingEnabled())
	assert.Equal(t, "evergreen.yml", repoRef.RemotePath)
	assert.Equal(t, "", repoRef.Identifier)
	assert.Nil(t, repoRef.NotifyOnBuildFailure)
	assert.Nil(t, repoRef.GithubChecksEnabled)
	assert.Equal(t, "my message", repoRef.CommitQueue.Message)

	assert.NotContains(t, repoRef.Admins, "bob")
	assert.NotContains(t, repoRef.Admins, "other bob")
	assert.Contains(t, repoRef.Admins, "me")
	users, err := user.FindByRole(t.Context(), GetRepoAdminRole(repoRef.Id))
	assert.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "me", users[0].Id)

	projectVars, err := FindOneProjectVars(t.Context(), repoRef.Id)
	assert.NoError(t, err)
	assert.Len(t, projectVars.Vars, 3)
	assert.Len(t, projectVars.PrivateVars, 1)
	assert.Equal(t, "world", projectVars.Vars["hello"])
	assert.Equal(t, "buggy", projectVars.Vars["sdc"])
	assert.Equal(t, "green", projectVars.Vars["ever"])
	assert.True(t, projectVars.PrivateVars["sdc"])

	projectAliases, err = FindAliasesForRepo(t.Context(), repoRef.Id)
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
	require.NoError(p.Insert(t.Context()))

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
		assert.NoError(p.ValidateGitHubPermissionGroupsByRequester())
	})

	t.Run("Invalid name in group fails validation", func(t *testing.T) {
		p.GitHubDynamicTokenPermissionGroups = append(orgGroup,
			GitHubDynamicTokenPermissionGroup{
				Name: "",
			},
		)
		assert.ErrorContains(p.ValidateGitHubPermissionGroupsByRequester(), "group name cannot be empty")
	})

	t.Run("Invalid requester in group fails validation", func(t *testing.T) {
		p.GitHubPermissionGroupByRequester = map[string]string{
			"second-requester": "some-group",
		}
		assert.ErrorContains(p.ValidateGitHubPermissionGroupsByRequester(), "requester 'second-requester' is not a valid requester")
	})

	t.Run("Valid requester pointing to not found group fails validation", func(t *testing.T) {
		p.GitHubPermissionGroupByRequester = map[string]string{
			evergreen.GithubPRRequester: "second-group",
		}
		assert.ErrorContains(p.ValidateGitHubPermissionGroupsByRequester(), fmt.Sprintf("group 'second-group' for requester '%s' not found", evergreen.GithubPRRequester))
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

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, fakeparameter.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(db.CreateCollections(evergreen.ScopeCollection))

	projectRef, err := FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "main", "")
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
	require.NoError(doc.Insert(t.Context()))

	// 1 disabled document = no match
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "main", "")
	assert.NoError(err)
	assert.Nil(projectRef)

	// 2 docs, 1 enabled, but the enabled one has pr testing disabled = no match
	doc.Id = "ident_"
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.Enabled = true
	require.NoError(doc.Insert(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "main", "")
	assert.NoError(err)
	require.Nil(projectRef)

	// 3 docs, 2 enabled, but only 1 has pr testing enabled = match
	doc.Id = "ident1"
	doc.PRTestingEnabled = utility.TruePtr()
	require.NoError(doc.Insert(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "main", "")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("ident1", projectRef.Id)

	// 2 matching documents, we just return one of those projects
	doc.Id = "ident2"
	require.NoError(doc.Insert(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "main", "")
	assert.NoError(err)
	assert.NotNil(projectRef)

	repoDoc := RepoRef{ProjectRef{
		Id:         "my_repo",
		Owner:      "mongodb",
		Repo:       "mci",
		RemotePath: "",
	}}
	assert.NoError(repoDoc.Replace(t.Context()))
	doc = &ProjectRef{
		Id:        "defaulting_project",
		Owner:     "mongodb",
		Repo:      "mci",
		Branch:    "mine",
		Enabled:   true,
		RepoRefId: repoDoc.Id,
	}
	assert.NoError(doc.Insert(t.Context()))
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
	assert.NoError(doc2.Insert(t.Context()))

	// repo doesn't have PR testing enabled, so no project returned
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.Nil(projectRef)

	repoDoc.PRTestingEnabled = utility.TruePtr()
	assert.NoError(repoDoc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", "")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("defaulting_project", projectRef.Id)

	// project PR testing explicitly disabled
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.ManualPRTestingEnabled = utility.FalsePtr()
	assert.NoError(doc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.Nil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", patch.AutomatedCaller)
	assert.NoError(err)
	assert.Nil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", patch.ManualCaller)
	assert.NoError(err)
	assert.Nil(projectRef)

	// project auto PR testing enabled, manual disabled
	doc.PRTestingEnabled = utility.TruePtr()
	doc.ManualPRTestingEnabled = utility.FalsePtr()
	assert.NoError(doc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.NotNil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", patch.AutomatedCaller)
	assert.NoError(err)
	assert.NotNil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", patch.ManualCaller)
	assert.NoError(err)
	assert.Nil(projectRef)

	// project auto PR testing disabled, manual enabled
	doc.PRTestingEnabled = utility.FalsePtr()
	doc.ManualPRTestingEnabled = utility.TruePtr()
	assert.NoError(doc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.NotNil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", patch.AutomatedCaller)
	assert.NoError(err)
	assert.Nil(projectRef)
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", patch.ManualCaller)
	assert.NoError(err)
	assert.NotNil(projectRef)

	// project explicitly disabled
	repoDoc.RemotePath = "my_path"
	assert.NoError(repoDoc.Replace(t.Context()))
	doc.Enabled = false
	doc.PRTestingEnabled = utility.TruePtr()
	assert.NoError(doc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "mine", "")
	assert.NoError(err)
	assert.Nil(projectRef)

	// branch with no project doesn't work and returns an error if repo not configured with a remote path
	repoDoc.RemotePath = ""
	assert.NoError(repoDoc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "yours", "")
	assert.Error(err)
	assert.Nil(projectRef)

	repoDoc.RemotePath = "my_path"
	assert.NoError(repoDoc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "yours", "")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("yours", projectRef.Branch)
	assert.True(projectRef.IsHidden())
	firstAttemptId := projectRef.Id

	// verify we return the same hidden project
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting(t.Context(), "mongodb", "mci", "yours", "")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal(firstAttemptId, projectRef.Id)
}

func TestFindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	projectRef, err := FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t.Context(), "mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:   "mongodb",
		Repo:    "mci",
		Branch:  "main",
		Id:      "mci",
		Enabled: true,
	}
	require.NoError(doc.Insert(t.Context()))

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t.Context(), "mongodb", "mci", "main")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc.CommitQueue.Enabled = utility.TruePtr()
	_, err = db.Replace(t.Context(), ProjectRefCollection, mgobson.M{ProjectRefIdKey: "mci"}, doc)
	require.NoError(err)

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t.Context(), "mongodb", "mci", "main")
	assert.NoError(err)
	assert.NotNil(projectRef)
	assert.Equal("mci", projectRef.Id)

	// doc doesn't default to repo
	doc.CommitQueue.Enabled = utility.FalsePtr()
	assert.NoError(doc.Replace(t.Context()))
	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t.Context(), "mongodb", "mci", "not_main")
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
	require.NoError(p1.Insert(t.Context()))
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
	require.NoError(p2.Insert(t.Context()))
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
	require.NoError(p3.Insert(t.Context()))
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
	require.NoError(p4.Insert(t.Context()))
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
	require.NoError(doc.Insert(t.Context()))
	ok, err := doc.CanEnableCommitQueue(t.Context())
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
	require.NoError(doc2.Insert(t.Context()))
	ok, err = doc2.CanEnableCommitQueue(t.Context())
	assert.NoError(err)
	assert.False(ok)
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
	require.NoError(t, proj1.Insert(t.Context()))

	proj2 := ProjectRef{
		Id:       "mci",
		Enabled:  false,
		Triggers: []TriggerDefinition{{Project: "grip"}},
	}
	require.NoError(t, proj2.Insert(t.Context()))

	projects, err := FindDownstreamProjects(t.Context(), "grip")
	assert.NoError(t, err)
	assert.Len(t, projects, 1)
	assert.Equal(t, proj1, projects[0])
}

func TestAddEmptyBranch(t *testing.T) {
	require.NoError(t, db.ClearCollections(user.Collection, ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	u := user.DBUser{
		Id: "me",
	}
	require.NoError(t, u.Insert(t.Context()))
	p := ProjectRef{
		Identifier: "myProject",
		Owner:      "mongodb",
		Repo:       "mongo",
	}
	assert.NoError(t, p.Add(t.Context(), &u))
	assert.NotEmpty(t, p.Id)
	assert.Empty(t, p.Branch)
	assert.Equal(t, []string{u.Id}, p.Admins)
}

func TestAddPermissions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(user.Collection, ProjectRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	env := testutil.NewEnvironment(ctx, t)
	u := user.DBUser{
		Id: "me",
	}
	assert.NoError(u.Insert(t.Context()))
	p := ProjectRef{
		Identifier: "myProject",
		Owner:      "mongodb",
		Repo:       "mongo",
		Branch:     "main",
		Hidden:     utility.TruePtr(),
	}
	assert.NoError(p.Add(t.Context(), &u))
	assert.NotEmpty(p.Id)
	assert.Equal([]string{u.Id}, p.Admins)
	assert.True(mgobson.IsObjectIdHex(p.Id))

	rm := env.RoleManager()
	scope, err := rm.FindScopeForResources(t.Context(), evergreen.ProjectResourceType, p.Id)
	assert.NoError(err)
	assert.NotNil(scope)
	role, err := rm.FindRoleWithPermissions(t.Context(), evergreen.ProjectResourceType, []string{p.Id}, map[string]int{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
		evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
		evergreen.PermissionLogs:            evergreen.LogsView.Value,
	})
	assert.NoError(err)
	assert.NotNil(role)
	dbUser, err := user.FindOneById(t.Context(), u.Id)
	assert.NoError(err)
	assert.Contains(dbUser.Roles(), fmt.Sprintf("admin_project_%s", p.Id))
	projectId := p.Id

	// check that an added project uses the hidden project's ID
	u = user.DBUser{Id: "you"}
	assert.NoError(u.Insert(t.Context()))
	p.Identifier = "differentProject"
	p.Id = ""
	assert.NoError(p.Add(t.Context(), &u))
	assert.NotEmpty(p.Id)
	assert.Contains(p.Admins, u.Id)
	assert.True(mgobson.IsObjectIdHex(p.Id))
	assert.Equal(projectId, p.Id)

	scope, err = rm.FindScopeForResources(t.Context(), evergreen.ProjectResourceType, p.Id)
	assert.NoError(err)
	assert.NotNil(scope)
	role, err = rm.FindRoleWithPermissions(t.Context(), evergreen.ProjectResourceType, []string{p.Id}, map[string]int{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
		evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
		evergreen.PermissionLogs:            evergreen.LogsView.Value,
	})
	assert.NoError(err)
	assert.NotNil(role)
	dbUser, err = user.FindOneById(t.Context(), u.Id)
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
	require.NoError(t, rm.AddScope(t.Context(), adminScope))
	adminRole := gimlet.Role{
		ID:          "admin",
		Scope:       evergreen.AllProjectsScope,
		Permissions: adminPermissions,
	}
	require.NoError(t, rm.UpdateRole(t.Context(), adminRole))
	oldAdmin := user.DBUser{
		Id:          "oldAdmin",
		SystemRoles: []string{"admin"},
	}
	require.NoError(t, oldAdmin.Insert(t.Context()))
	newAdmin := user.DBUser{
		Id: "newAdmin",
	}
	require.NoError(t, newAdmin.Insert(t.Context()))
	p := ProjectRef{
		Id: "proj",
	}
	require.NoError(t, p.Insert(t.Context()))

	modified, err := p.UpdateAdminRoles(t.Context(), []string{newAdmin.Id}, []string{oldAdmin.Id})
	assert.NoError(t, err)
	assert.True(t, modified)
	oldAdminFromDB, err := user.FindOneById(t.Context(), oldAdmin.Id)
	assert.NoError(t, err)
	assert.Empty(t, oldAdminFromDB.Roles())
	newAdminFromDB, err := user.FindOneById(t.Context(), newAdmin.Id)
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
	require.NoError(t, oldAdmin.Insert(t.Context()))
	newAdmin := user.DBUser{
		Id: "newAdmin",
	}
	require.NoError(t, newAdmin.Insert(t.Context()))
	p := ProjectRef{
		Id:     "proj",
		Admins: []string{oldAdmin.Id},
	}
	require.NoError(t, p.Insert(t.Context()))

	// check that, without a valid role, the whole update fails
	modified, err := p.UpdateAdminRoles(t.Context(), []string{"nonexistent-user", newAdmin.Id}, []string{"nonexistent-user", oldAdmin.Id})
	assert.Error(t, err)
	assert.False(t, modified)
	assert.Equal(t, []string{oldAdmin.Id}, p.Admins)

	rm := env.RoleManager()
	adminScope := gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"proj"},
	}
	require.NoError(t, rm.AddScope(t.Context(), adminScope))
	adminRole := gimlet.Role{
		ID:          "admin",
		Scope:       evergreen.AllProjectsScope,
		Permissions: adminPermissions,
	}
	require.NoError(t, rm.UpdateRole(t.Context(), adminRole))

	// check that the existing users have been added and removed while returning an error
	modified, err = p.UpdateAdminRoles(t.Context(), []string{"nonexistent-user", newAdmin.Id}, []string{"nonexistent-user", oldAdmin.Id})
	assert.Error(t, err)
	assert.True(t, modified)
	oldAdminFromDB, err := user.FindOneById(t.Context(), oldAdmin.Id)
	assert.NoError(t, err)
	assert.Empty(t, oldAdminFromDB.Roles())
	newAdminFromDB, err := user.FindOneById(t.Context(), newAdmin.Id)
	assert.NoError(t, err)
	assert.Len(t, newAdminFromDB.Roles(), 1)
}

func TestGetProjectTasksWithOptions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection, ProjectRefCollection, RepositoriesCollection))
	p := ProjectRef{
		Id:         "my_project",
		Identifier: "my_ident",
	}
	assert.NoError(t, p.Insert(t.Context()))
	assert.NoError(t, db.Insert(t.Context(), RepositoriesCollection, Repository{
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
		assert.NoError(t, myTask.Insert(t.Context()))
	}
	opts := GetProjectTasksOpts{}

	tasks, err := GetTasksWithOptions(t.Context(), "my_ident", "t1", opts)
	assert.NoError(t, err)
	// Returns 7 tasks because 40 tasks exist within the default version limit,
	// but 1/2 are undispatched and only 1/3 have a system requester
	assert.Len(t, tasks, 7)

	opts.Limit = 5
	tasks, err = GetTasksWithOptions(t.Context(), "my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.Equal(t, 99, tasks[0].RevisionOrderNumber)
	assert.Equal(t, 96, tasks[1].RevisionOrderNumber)

	opts.Limit = 10
	opts.StartAt = 80
	tasks, err = GetTasksWithOptions(t.Context(), "my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 3)
	assert.Equal(t, 78, tasks[0].RevisionOrderNumber)
	assert.Equal(t, 72, tasks[2].RevisionOrderNumber)

	opts.Requesters = []string{evergreen.PatchVersionRequester}
	tasks, err = GetTasksWithOptions(t.Context(), "my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 7)
	assert.Equal(t, 80, tasks[0].RevisionOrderNumber)
	assert.Equal(t, 71, tasks[6].RevisionOrderNumber)

	opts.Requesters = []string{evergreen.RepotrackerVersionRequester}
	tasks, err = GetTasksWithOptions(t.Context(), "my_ident", "t1", opts)
	assert.NoError(t, err)
	assert.Len(t, tasks, 3)
	assert.Equal(t, 78, tasks[0].RevisionOrderNumber)
	assert.Equal(t, 72, tasks[2].RevisionOrderNumber)

	opts.Requesters = []string{}
	opts.Limit = defaultVersionLimit
	opts.StartAt = 90
	opts.BuildVariant = "bv1"
	tasks, err = GetTasksWithOptions(t.Context(), "my_ident", "t1", opts)
	// Returns 7 tasks because 40 tasks exist within the default version limit,
	// but only 1/6 matches the bv and is not undispatched
	assert.NoError(t, err)
	assert.Len(t, tasks, 7)
	assert.Equal(t, 90, tasks[0].RevisionOrderNumber)
	assert.Equal(t, 72, tasks[6].RevisionOrderNumber)
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
			assert.NoError(p.Insert(t.Context()))
			assert.NoError(repoRef.Replace(t.Context()))

			assert.NoError(UpdateNextPeriodicBuild(t.Context(), "proj", &p.PeriodicBuilds[1]))
			dbProject, err := FindBranchProjectRef(t.Context(), p.Id)
			assert.NoError(err)
			require.NotNil(dbProject)
			assert.True(now.Equal(dbProject.PeriodicBuilds[0].NextRunTime))
			assert.True(muchLater.Equal(dbProject.PeriodicBuilds[1].NextRunTime))

			dbRepo, err := FindOneRepoRef(t.Context(), p.RepoRefId)
			assert.NoError(err)
			require.NotNil(dbRepo)
			// Repo wasn't updated because the branch project definitions take precedent.
			assert.True(later.Equal(dbRepo.PeriodicBuilds[0].NextRunTime))
		},
		"updatesProjectFromThePast": func(t *testing.T) {
			p := ProjectRef{
				Id: "proj",
				PeriodicBuilds: []PeriodicBuildDefinition{
					{ID: "0", NextRunTime: now.Add(-48 * time.Hour), IntervalHours: 5},
				},
			}
			assert.NoError(p.Insert(t.Context()))

			assert.NoError(UpdateNextPeriodicBuild(t.Context(), "proj", &p.PeriodicBuilds[0]))
			dbProject, err := FindBranchProjectRef(t.Context(), p.Id)
			assert.NoError(err)
			require.NotNil(dbProject)
			nextRunTime := now.Add(5 * time.Hour).Round(time.Hour)
			assert.True(nextRunTime.Equal(dbProject.PeriodicBuilds[0].NextRunTime.Round(time.Hour)))
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
			assert.NoError(p.Insert(t.Context()))
			assert.NoError(repoRef.Replace(t.Context()))
			assert.NoError(UpdateNextPeriodicBuild(t.Context(), "proj", &repoRef.PeriodicBuilds[0]))

			// Repo is updated because the branch project doesn't have any periodic build override defined.
			dbRepo, err := FindOneRepoRef(t.Context(), p.RepoRefId)
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
			assert.NoError(p.Insert(t.Context()))
			assert.NoError(repoRef.Replace(t.Context()))
			// Should error because definition isn't relevant for this project, since
			// we ignore repo definitions when the project has any override defined.
			assert.Error(UpdateNextPeriodicBuild(t.Context(), "proj", &repoRef.PeriodicBuilds[0]))

			dbRepo, err := FindOneRepoRef(t.Context(), p.RepoRefId)
			assert.NoError(err)
			assert.NotNil(dbRepo)
			assert.True(later.Equal(dbRepo.PeriodicBuilds[0].NextRunTime))
		},
		"updateProjectWithCron": func(t *testing.T) {
			nextRunTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
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
			assert.NoError(p.Insert(t.Context()))
			assert.NoError(repoRef.Replace(t.Context()))

			assert.NoError(UpdateNextPeriodicBuild(t.Context(), "proj", &p.PeriodicBuilds[0]))
			dbProject, err := FindBranchProjectRef(t.Context(), p.Id)
			assert.NoError(err)
			require.NotNil(dbProject)
			assert.True(nextDay.Equal(dbProject.PeriodicBuilds[0].NextRunTime))
			assert.True(laterRunTime.Equal(dbProject.PeriodicBuilds[1].NextRunTime))

			// Even with a different runtime we get the same result, since we're using a cron.
			assert.NoError(UpdateNextPeriodicBuild(t.Context(), "proj", &p.PeriodicBuilds[1]))
			dbProject, err = FindBranchProjectRef(t.Context(), p.Id)
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
	assert.NoError(t, projectRef.Insert(t.Context()))

	assert.NotPanics(t, func() {
		_, err = FindAnyRestrictedProjectRef(t.Context())
	}, "Should not panic if there are no matching projects")
	assert.Error(t, err, "Should return error if there are no matching projects")

	projectRef = ProjectRef{
		Id:      "p1",
		Enabled: true,
	}
	assert.NoError(t, projectRef.Insert(t.Context()))

	resultRef, err := FindAnyRestrictedProjectRef(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, "p1", resultRef.Id)
}

func TestFindPeriodicProjects(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection))

	repoRef := RepoRef{ProjectRef{
		Id:             "my_repo",
		PeriodicBuilds: []PeriodicBuildDefinition{{ID: "repo_def"}},
	}}
	assert.NoError(t, repoRef.Replace(t.Context()))

	pRef := ProjectRef{
		Id:             "p1",
		RepoRefId:      "my_repo",
		Enabled:        true,
		PeriodicBuilds: []PeriodicBuildDefinition{},
	}
	assert.NoError(t, pRef.Insert(t.Context()))

	pRef.Id = "p2"
	pRef.Enabled = true
	pRef.PeriodicBuilds = []PeriodicBuildDefinition{{ID: "p1"}}
	assert.NoError(t, pRef.Insert(t.Context()))

	pRef.Id = "p3"
	pRef.Enabled = true
	pRef.PeriodicBuilds = nil
	assert.NoError(t, pRef.Insert(t.Context()))

	pRef.Id = "p4"
	pRef.Enabled = false
	pRef.PeriodicBuilds = []PeriodicBuildDefinition{{ID: "p1"}}
	assert.NoError(t, pRef.Insert(t.Context()))

	projects, err := FindPeriodicProjects(t.Context())
	assert.NoError(t, err)
	assert.Len(t, projects, 2)
	for _, p := range projects {
		assert.Len(t, p.PeriodicBuilds, 1, "project '%s' missing definition", p.Id)
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

	assert.NoError(t, pRef.Replace(t.Context()))
	assert.NoError(t, pRef2.Replace(t.Context()))
	assert.NoError(t, pRef3.Replace(t.Context()))
	assert.NoError(t, repoRef.Replace(t.Context()))
	assert.NoError(t, repoRef2.Replace(t.Context()))
	assert.NoError(t, repoRef3.Replace(t.Context()))

	assert.NoError(t, RemoveAdminFromProjects(t.Context(), "villain"))

	// verify that we carry out multiple updates
	pRefFromDB, err := FindBranchProjectRef(t.Context(), pRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")
	pRefFromDB, err = FindBranchProjectRef(t.Context(), pRef2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")
	pRefFromDB, err = FindBranchProjectRef(t.Context(), pRef3.Id)
	assert.NoError(t, err)
	assert.NotNil(t, pRefFromDB)
	assert.NotContains(t, pRefFromDB.Admins, "villain")

	repoRefFromDB, err := FindOneRepoRef(t.Context(), repoRef.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
	repoRefFromDB, err = FindOneRepoRef(t.Context(), repoRef2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, repoRefFromDB)
	assert.NotContains(t, repoRefFromDB.Admins, "villain")
	repoRefFromDB, err = FindOneRepoRef(t.Context(), repoRef3.Id)
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

	assert.NoError(t, db.Insert(t.Context(), ProjectRefCollection, ref))

	pointerRef := struct {
		PtrString *string            `bson:"my_str"`
		PtrBool   *bool              `bson:"my_bool"`
		PtrStruct *WorkstationConfig `bson:"config"`
	}{}
	assert.NoError(t, db.FindOneQ(t.Context(), ProjectRefCollection, db.Query(bson.M{}), &pointerRef))
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
			BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketCreateIssueType:   "Bug",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			},
			GithubPRTriggerAliases: []string{"one", "two"},
			GithubMQTriggerAliases: []string{"three", "four"},
		},
	}
	assert.NoError(t, projectRef.Insert(t.Context()))
	assert.NoError(t, projectConfig.Insert(t.Context()))

	err := projectRef.MergeWithProjectConfig(t.Context(), "version1")
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
	assert.Equal(t, []string{"one", "two"}, projectRef.GithubPRTriggerAliases)
	assert.Equal(t, []string{"three", "four"}, projectRef.GithubMQTriggerAliases)
	assert.Equal(t, "p1", projectRef.PeriodicBuilds[0].ID)
	err = projectRef.MergeWithProjectConfig(t.Context(), "version1")
	assert.NoError(t, err)
	require.NotNil(t, projectRef)
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
	assert.NoError(projectRef.Insert(t.Context()))
	projectRef, err := FindBranchProjectRef(t.Context(), "identifier")
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
	_, err = SaveProjectPageForSection(t.Context(), "iden_", update, ProjectPageGeneralSection, false)
	assert.NoError(err)

	// Verify that Parsley filters and project health view are saved correctly.
	update = &ProjectRef{
		ParsleyFilters: []parsley.Filter{
			{Expression: "filter", CaseSensitive: false, ExactMatch: true},
		},
		ProjectHealthView: ProjectHealthViewAll,
	}
	_, err = SaveProjectPageForSection(t.Context(), "iden_", update, ProjectPageViewsAndFiltersSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef(t.Context(), "iden_")
	assert.NoError(err)
	require.NotNil(t, projectRef)
	assert.Len(projectRef.ParsleyFilters, 1)
	assert.Equal(ProjectHealthViewAll, projectRef.ProjectHealthView)

	// Verify that private field does not get updated when updating restricted field.
	update = &ProjectRef{
		Restricted: utility.TruePtr(),
	}
	_, err = SaveProjectPageForSection(t.Context(), "iden_", update, ProjectPageAccessSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef(t.Context(), "iden_")
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
	_, err = SaveProjectPageForSection(t.Context(), "iden_", update, ProjectPageGithubPermissionsSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef(t.Context(), "iden_")
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
	_, err = SaveProjectPageForSection(t.Context(), "iden_", update, ProjectPageGithubAppSettingsSection, false)
	assert.NoError(err)

	projectRef, err = FindBranchProjectRef(t.Context(), "iden_")
	require.NoError(t, err)
	require.NotNil(t, projectRef)
	require.NotNil(t, projectRef.GitHubPermissionGroupByRequester)
	assert.Len(projectRef.GitHubPermissionGroupByRequester, 1)
	assert.Equal("some-group", projectRef.GitHubPermissionGroupByRequester[evergreen.PatchVersionRequester])
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
	require.NoError(t, project.Insert(t.Context()))

	err := project.ValidateOwnerAndRepo([]string{"evergreen-ci"})
	assert.Error(t, err)

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
	require.NoError(t, pRef.Insert(t.Context()))
	t.Run("OverwritesError", func(t *testing.T) {
		repotrackerErr := &RepositoryErrorDetails{
			Exists:            true,
			InvalidRevision:   "invalid_revision",
			MergeBaseRevision: "merge_base_revision",
		}
		require.NoError(t, pRef.SetRepotrackerError(t.Context(), repotrackerErr))
		dbProjRef, err := FindBranchProjectRef(t.Context(), pRef.Identifier)
		require.NoError(t, err)
		require.NotZero(t, dbProjRef)
		require.NotZero(t, dbProjRef.RepotrackerError)
		assert.Equal(t, *repotrackerErr, *dbProjRef.RepotrackerError)
	})
	t.Run("ClearsError", func(t *testing.T) {
		require.NoError(t, pRef.SetRepotrackerError(t.Context(), &RepositoryErrorDetails{}))
		dbProjRef, err := FindBranchProjectRef(t.Context(), pRef.Identifier)
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
	assert.NoError(projectRef.Insert(t.Context()))

	// Set based on last activation time when no version is found
	versionCreatedAt := time.Now().Add(-1 * time.Minute)
	activationTime, err := projectRef.GetActivationTimeForVariant(t.Context(), &BuildVariant{Name: "bv"}, false, versionCreatedAt, time.Now())
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
	assert.NoError(version.Insert(t.Context()))

	activationTime, err = projectRef.GetActivationTimeForVariant(t.Context(), &BuildVariant{Name: "bv"}, false, versionCreatedAt, time.Now())
	assert.NoError(err)
	assert.False(utility.IsZeroTime(activationTime))
	assert.Equal(activationTime, versionCreatedAt)
}

func TestGetActivationTimeWithPathFiltering(t *testing.T) {
	setup := func(t *testing.T) (*ProjectRef, time.Time) {
		require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection))
		projectRef := &ProjectRef{
			Owner:      "mongodb",
			Repo:       "mci",
			Branch:     "main",
			Enabled:    true,
			Id:         "ident",
			Identifier: "identifier",
		}
		require.NoError(t, projectRef.Insert(t.Context()))
		versionCreatedAt := time.Now().Add(-1 * time.Minute)
		return projectRef, versionCreatedAt
	}

	t.Run("PathFilteredNoCronBatchtimeActivation", func(t *testing.T) {
		projectRef, _ := setup(t)
		// If the variant paths are filtered and no cron/batchtime/activation is set, the activation time should be zero
		activationTime, err := projectRef.GetActivationTimeForVariant(t.Context(), &BuildVariant{Name: "bv"}, true, time.Now(), time.Now())
		assert.NoError(t, err)
		assert.True(t, utility.IsZeroTime(activationTime))
	})

	t.Run("PathFilteredWithCron", func(t *testing.T) {
		projectRef, versionCreatedAt := setup(t)
		// If the variant paths are filtered but cron is set, the activation time should be non-zero
		bv := BuildVariant{
			Name:          "bv",
			CronBatchTime: "@daily",
		}
		activationTime, err := projectRef.GetActivationTimeForVariant(t.Context(), &bv, true, versionCreatedAt, time.Now())
		assert.NoError(t, err)
		assert.False(t, utility.IsZeroTime(activationTime))
	})

	t.Run("PathFilteredWithBatchtime", func(t *testing.T) {
		projectRef, versionCreatedAt := setup(t)
		// If the variant paths are filtered but batchtime is set, the activation time should be non-zero
		bv := BuildVariant{
			Name:      "bv",
			BatchTime: utility.ToIntPtr(10),
		}
		activationTime, err := projectRef.GetActivationTimeForVariant(t.Context(), &bv, true, versionCreatedAt, time.Now())
		assert.NoError(t, err)
		assert.False(t, utility.IsZeroTime(activationTime))
	})

	t.Run("PathFilteredWithActivate", func(t *testing.T) {
		projectRef, versionCreatedAt := setup(t)
		// If the variant paths are filtered but activate is set, the activation time should be non-zero
		bv := BuildVariant{
			Name:     "bv",
			Activate: utility.TruePtr(),
		}
		activationTime, err := projectRef.GetActivationTimeForVariant(t.Context(), &bv, true, versionCreatedAt, time.Now())
		assert.NoError(t, err)
		assert.False(t, utility.IsZeroTime(activationTime))
	})

	t.Run("PathFilteredWithBatchtimeAndActivate", func(t *testing.T) {
		projectRef, versionCreatedAt := setup(t)
		// If the variant has both batchtime and activate set, the activation time should be non-zero
		bv := BuildVariant{
			Name:      "bv",
			BatchTime: utility.ToIntPtr(10),
			Activate:  utility.TruePtr(),
		}
		activationTime, err := projectRef.GetActivationTimeForVariant(t.Context(), &bv, true, versionCreatedAt, time.Now())
		assert.NoError(t, err)
		assert.False(t, utility.IsZeroTime(activationTime))
		// When activate is true, the activation time should be the version creation time
		assert.Equal(t, versionCreatedAt, activationTime)
	})
}

func TestActivationTimeWithDuplicate(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection))

	now := time.Date(2025, time.May, 7, 0, 1, 0, 0, time.UTC)                    // 5/7 12:01 AM
	versionCreateTime := time.Date(2025, time.May, 6, 23, 59, 0, 0, time.UTC)    // 5/6 11:59 PM
	prevVersionCreateTime := time.Date(2025, time.May, 6, 16, 0, 0, 0, time.UTC) // 5/6 4:00 PM
	midnightActivateTime := time.Date(2025, time.May, 7, 0, 0, 0, 0, time.UTC)   // 5/7 12:00 AM

	// Set up project
	projectRef := &ProjectRef{
		Id:         "myproj",
		Identifier: "myproj",
		Enabled:    true,
	}
	require.NoError(t, projectRef.Insert(t.Context()))

	// A previous version created at 4pm with activation time at midnight
	prevVersion := &Version{
		Id:                  "prev",
		Identifier:          "myproj",
		CreateTime:          prevVersionCreateTime,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "bv",
				ActivationStatus: ActivationStatus{
					Activated:  false,
					ActivateAt: midnightActivateTime, // scheduled to run at midnight
				},
			},
		},
	}
	require.NoError(t, prevVersion.Insert(t.Context()))

	// Create build variant with midnight cron
	bv := BuildVariant{
		Name:          "bv",
		CronBatchTime: "0 0 * * *", // midnight every day
	}

	activationTime, err := projectRef.GetActivationTimeForVariant(t.Context(), &bv, false, versionCreateTime, now)
	require.NoError(t, err)

	// get the previous version to check the activation time
	dbVersion, err := VersionFindOneId(t.Context(), prevVersion.Id)
	// Check if the version was found
	require.NoError(t, err)
	require.NotNil(t, dbVersion)

	// Since the previous version is scheduled to run at midnight and our new version was created at 11:59pm,
	// because it's within the five minute window, it should be scheduled for the next midnight instead of
	// this midnight.
	nextMidnight := midnightActivateTime.Add(24 * time.Hour)
	assert.Equal(t, nextMidnight, activationTime)
}

func TestUserHasRepoViewPermission(t *testing.T) {
	wrongProjectScopeId := "wrongProjectScope"
	projectScopeId := "projectScope"

	for testName, testCase := range map[string]func(t *testing.T, usr *user.DBUser, roleManager gimlet.RoleManager){
		"wrongProjectViewPermissionFails": func(t *testing.T, usr *user.DBUser, roleManager gimlet.RoleManager) {
			wrongProjectRole := gimlet.Role{
				ID:          "view_branch_role",
				Scope:       wrongProjectScopeId,
				Permissions: map[string]int{evergreen.PermissionProjectSettings: 20},
			}
			require.NoError(t, roleManager.UpdateRole(t.Context(), wrongProjectRole))

			assert.NoError(t, usr.AddRole(t.Context(), wrongProjectRole.ID))
			hasPermission, err := UserHasRepoViewPermission(t.Context(), usr, "myRepoId")
			assert.NoError(t, err)
			assert.False(t, hasPermission)
		},
		"wrongPermissionViewPermissionFails": func(t *testing.T, usr *user.DBUser, roleManager gimlet.RoleManager) {
			wrongPermissionRole := gimlet.Role{
				ID:          "view_branch_role",
				Scope:       projectScopeId,
				Permissions: map[string]int{evergreen.PermissionTasks: 30},
			}
			require.NoError(t, roleManager.UpdateRole(t.Context(), wrongPermissionRole))

			assert.NoError(t, usr.AddRole(t.Context(), wrongPermissionRole.ID))
			hasPermission, err := UserHasRepoViewPermission(t.Context(), usr, "myRepoId")
			assert.NoError(t, err)
			assert.False(t, hasPermission)
		},
		"branchViewPermissionSucceeds": func(t *testing.T, usr *user.DBUser, roleManager gimlet.RoleManager) {
			viewBranchRole := gimlet.Role{
				ID:          "view_branch_role",
				Scope:       projectScopeId,
				Permissions: map[string]int{evergreen.PermissionProjectSettings: 20},
			}
			require.NoError(t, roleManager.UpdateRole(t.Context(), viewBranchRole))

			assert.NoError(t, usr.AddRole(t.Context(), viewBranchRole.ID))
			hasPermission, err := UserHasRepoViewPermission(t.Context(), usr, "myRepoId")
			assert.NoError(t, err)
			assert.True(t, hasPermission)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectRefCollection, user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection))
			pRef := ProjectRef{
				Id:        "project1",
				RepoRefId: "myRepoId",
			}
			wrongRef := ProjectRef{
				Id: "project2",
			}
			assert.NoError(t, pRef.Insert(t.Context()))
			assert.NoError(t, wrongRef.Insert(t.Context()))
			env := evergreen.GetEnvironment()
			roleManager := env.RoleManager()
			projectScope := gimlet.Scope{
				ID:        projectScopeId,
				Type:      evergreen.ProjectResourceType,
				Resources: []string{pRef.Id},
			}
			assert.NoError(t, roleManager.AddScope(t.Context(), projectScope))
			wrongProjectScope := gimlet.Scope{
				ID:        wrongProjectScopeId,
				Type:      evergreen.ProjectResourceType,
				Resources: []string{wrongRef.Id},
			}
			assert.NoError(t, roleManager.AddScope(t.Context(), wrongProjectScope))

			usr := &user.DBUser{Id: "usr"}
			assert.NoError(t, usr.Insert(t.Context()))
			testCase(t, usr, roleManager)
		})
	}
}
