package validator

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
)

func tagValidationTestTask(name string, tags ...string) model.ProjectTask {
	return model.ProjectTask{
		Name: name,
		Tags: tags,
	}
}

func tagValidationTestBV(name string, tasks ...model.BuildVariantTaskUnit) model.BuildVariant {
	return model.BuildVariant{
		Name:  name,
		Tasks: tasks,
	}
}

func TestCheckGeneratedTaskTagValidation(t *testing.T) {
	const prefix = "owning_team_"
	const prefixedTag = "owning_team_data"

	// If a retried generate.tasks job hits the UpdatedByGenerators short-circuit in
	// GeneratedProject.NewVersion (model/generate.go), the merged project returned is the
	// same pointer as the pre-existing project, and the diff should find nothing new.
	unchangedProject := &model.Project{
		Tasks: []model.ProjectTask{
			tagValidationTestTask("new-task", "unrelated_tag"),
		},
		BuildVariants: []model.BuildVariant{
			tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "new-task"}),
		},
	}

	for testName, testCase := range map[string]struct {
		existing        *model.Project
		merged          *model.Project
		prefix          string
		expectedFailing []TagValidationFailure
	}{
		"NewTaskWithRequiredTagPrefixPasses": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("new-task", prefixedTag),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "new-task"}),
				},
			},
			prefix: prefix,
		},
		"NewTaskWithoutRequiredTagPrefixFails": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("new-task", "unrelated_tag"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "new-task"}),
				},
			},
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "new-task", BuildVariants: []string{"variant"}}},
		},
		"NewTaskWithNoTagsFails": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("new-task"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "new-task"}),
				},
			},
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "new-task", BuildVariants: []string{"variant"}}},
		},
		"NewTaskPassesIfAnyTagHasRequiredPrefix": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("new-task", "unrelated_tag", prefixedTag),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "new-task"}),
				},
			},
			prefix: prefix,
		},
		"VariantTagDoesNotSatisfyValidation": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("new-task"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "new-task"}),
				},
			},
			// The merged variant has the required prefix as a variant tag, but variant tags
			// do not satisfy validation, so the task still fails.
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "new-task", BuildVariants: []string{"variant"}}},
		},
		"ExistingTaskAddedToVariantIsCheckedAgainstItsOwnTags": {
			existing: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("static-task", "unrelated_tag"),
					tagValidationTestTask("other-task", prefixedTag),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("existing-variant", model.BuildVariantTaskUnit{Name: "other-task"}),
				},
			},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("static-task", "unrelated_tag"),
					tagValidationTestTask("other-task", prefixedTag),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("existing-variant",
						model.BuildVariantTaskUnit{Name: "other-task"},
						model.BuildVariantTaskUnit{Name: "static-task"}),
				},
			},
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "static-task", BuildVariants: []string{"existing-variant"}}},
		},
		"ExistingTaskAlreadyInVariantIsIgnored": {
			existing: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("static-task", "unrelated_tag"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "static-task"}),
				},
			},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("static-task", "unrelated_tag"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "static-task"}),
				},
			},
			prefix: prefix,
		},
		"ExistingTaskAddedToNewVariantIsChecked": {
			existing: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("static-task", "unrelated_tag"),
				},
			},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("static-task", "unrelated_tag"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("new-variant", model.BuildVariantTaskUnit{Name: "static-task"}),
				},
			},
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "static-task", BuildVariants: []string{"new-variant"}}},
		},
		"GeneratedTaskGroupMemberWithoutRequiredTagPrefixFails": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("failing-member"),
					tagValidationTestTask("passing-member", prefixedTag),
				},
				TaskGroups: []model.TaskGroup{
					{
						Name: "task-group",
						Tags: []string{prefixedTag},
						Tasks: []string{
							"failing-member",
							"passing-member",
						},
					},
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "task-group", IsGroup: true}),
				},
			},
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "failing-member", BuildVariants: []string{"variant"}}},
		},
		"ExistingTaskGroupAddedToVariantIsChecked": {
			existing: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("group-member", "unrelated_tag"),
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "task-group",
						Tasks: []string{"group-member"},
					},
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("existing-variant", model.BuildVariantTaskUnit{Name: "other-task"}),
				},
			},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("group-member", "unrelated_tag"),
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "task-group",
						Tasks: []string{"group-member"},
					},
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("existing-variant",
						model.BuildVariantTaskUnit{Name: "other-task"},
						model.BuildVariantTaskUnit{Name: "task-group", IsGroup: true}),
				},
			},
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "group-member", BuildVariants: []string{"existing-variant"}}},
		},
		"TaskFailingInMultipleVariantsIsReportedOnce": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("new-task", "unrelated_tag"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant-1", model.BuildVariantTaskUnit{Name: "new-task"}),
					tagValidationTestBV("variant-2", model.BuildVariantTaskUnit{Name: "new-task"}),
				},
			},
			prefix:          prefix,
			expectedFailing: []TagValidationFailure{{TaskName: "new-task", BuildVariants: []string{"variant-1", "variant-2"}}},
		},
		"MergedSameAsExistingReturnsNoFailures": {
			existing: unchangedProject,
			merged:   unchangedProject,
			prefix:   prefix,
		},
		"EmptyPrefixReturnsNoFailures": {
			existing: &model.Project{},
			merged: &model.Project{
				Tasks: []model.ProjectTask{
					tagValidationTestTask("new-task", "unrelated_tag"),
				},
				BuildVariants: []model.BuildVariant{
					tagValidationTestBV("variant", model.BuildVariantTaskUnit{Name: "new-task"}),
				},
			},
			prefix: "",
		},
	} {
		t.Run(testName, func(t *testing.T) {
			failingTasks := CheckGeneratedTaskTagValidation(testCase.existing, testCase.merged, testCase.prefix)
			assert.Equal(t, testCase.expectedFailing, failingTasks)
		})
	}
}
