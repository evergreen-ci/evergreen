package validator

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/testutil"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	tu "github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

var projectValidatorConf = tu.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(projectValidatorConf))
}

func TestVerifyTaskDependencies(t *testing.T) {
	Convey("When validating a project's dependencies", t, func() {
		Convey("if any task has a duplicate dependency, an error should be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile"},
							{Name: "compile"},
						},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldNotResemble, []ValidationError{})
			So(len(verifyTaskDependencies(project)), ShouldEqual, 1)
		})

		Convey("no error should be returned for dependencies of the same task on two variants", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile", Variant: "v1"},
							{Name: "compile", Variant: "v2"},
						},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldResemble, []ValidationError{})
			So(len(verifyTaskDependencies(project)), ShouldEqual, 0)
		})

		Convey("if any dependencies have an invalid name field, an error should be returned", func() {

			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "bad"}},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldNotResemble, []ValidationError{})
			So(len(verifyTaskDependencies(project)), ShouldEqual, 1)
		})

		Convey("if any dependencies have an invalid status field, an error should be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "compile", Status: "flibbertyjibbit"}},
					},
					{
						Name:      "testTwo",
						DependsOn: []model.TaskDependency{{Name: "compile", Status: evergreen.TaskSucceeded}},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldNotResemble, []ValidationError{})
			So(len(verifyTaskDependencies(project)), ShouldEqual, 1)
		})

		Convey("if the dependencies are well-formed, no error should be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "compile"}},
					},
					{
						Name:      "testTwo",
						DependsOn: []model.TaskDependency{{Name: "compile"}},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestCheckDependencyGraph(t *testing.T) {
	Convey("When checking a project's dependency graph", t, func() {
		Convey("cycles in the dependency graph should cause error to be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{{Name: "testOne"}},
					},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "compile"}},
					},
					{
						Name:      "testTwo",
						DependsOn: []model.TaskDependency{{Name: "compile"}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}, {Name: "testTwo"}},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 3)
		})

		Convey("task wildcard cycles in the dependency graph should return an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "compile"}, {Name: "testTwo"}},
					},
					{
						Name:      "testTwo",
						DependsOn: []model.TaskDependency{{Name: model.AllDependencies}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}, {Name: "testTwo"}},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 2)
		})

		Convey("nonexisting nodes in the dependency graph should return an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "compile"}, {Name: "hamSteak"}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 1)
		})

		Convey("cross-variant cycles in the dependency graph should return an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
					},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile"},
							{Name: "testSpecial", Variant: "bv2"},
						},
					},
					{
						Name:      "testSpecial",
						DependsOn: []model.TaskDependency{{Name: "testOne", Variant: "bv1"}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
					{
						Name:  "bv2",
						Tasks: []model.BuildVariantTask{{Name: "testSpecial"}}},
				},
			}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 2)
		})

		Convey("cycles/errors from overwriting the dependency graph should return an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
					},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile"},
						},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTask{
							{Name: "compile", DependsOn: []model.TaskDependency{{Name: "testOne"}}},
							{Name: "testOne"},
						},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 2)

			project.BuildVariants[0].Tasks[0].DependsOn = nil
			project.BuildVariants[0].Tasks[1].DependsOn = []model.TaskDependency{{Name: "NOPE"}}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 1)

			project.BuildVariants[0].Tasks[1].DependsOn = []model.TaskDependency{{Name: "compile", Variant: "bvNOPE"}}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 1)
		})

		Convey("variant wildcard cycles in the dependency graph should return an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
					},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile"},
							{Name: "testSpecial", Variant: "bv2"},
						},
					},
					{
						Name:      "testSpecial",
						DependsOn: []model.TaskDependency{{Name: "testOne", Variant: model.AllVariants}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTask{
							{Name: "testSpecial"}},
					},
					{
						Name: "bv3",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
					{
						Name: "bv4",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 4)
		})

		Convey("cycles in a ** dependency graph should return an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile", Variant: model.AllVariants},
							{Name: "testTwo"},
						},
					},
					{
						Name: "testTwo",
						DependsOn: []model.TaskDependency{
							{Name: model.AllDependencies, Variant: model.AllVariants},
						},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}, {Name: "testTwo"}},
					},
				},
			}

			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 3)
		})

		Convey("if any task has itself as a dependency, an error should be"+
			" returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "testOne"}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name:  "bv",
						Tasks: []model.BuildVariantTask{{Name: "compile"}, {Name: "testOne"}},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldNotResemble, []ValidationError{})
			So(len(checkDependencyGraph(project)), ShouldEqual, 1)
		})

		Convey("if there is no cycle in the dependency graph, no error should"+
			" be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					{
						Name:      "testOne",
						DependsOn: []model.TaskDependency{{Name: "compile"}},
					},
					{
						Name:      "testTwo",
						DependsOn: []model.TaskDependency{{Name: "compile"}},
					},
					{
						Name: "push",
						DependsOn: []model.TaskDependency{
							{Name: "testOne"},
							{Name: "testTwo"},
						},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}, {Name: "testTwo"}},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldResemble, []ValidationError{})
		})

		Convey("if there is no cycle in the cross-variant dependency graph, no error should"+
			" be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile", Variant: "bv2"},
						},
					},
					{
						Name: "testSpecial",
						DependsOn: []model.TaskDependency{
							{Name: "compile"},
							{Name: "testOne", Variant: "bv1"}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTask{
							{Name: "testOne"}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testSpecial"}},
					},
				},
			}

			So(checkDependencyGraph(project), ShouldResemble, []ValidationError{})
		})

		Convey("if there is no cycle in the * dependency graph, no error should be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile", Variant: model.AllVariants},
						},
					},
					{
						Name:      "testTwo",
						DependsOn: []model.TaskDependency{{Name: model.AllDependencies}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testTwo"}},
					},
				},
			}

			So(checkDependencyGraph(project), ShouldResemble, []ValidationError{})
		})

		Convey("if there is no cycle in the ** dependency graph, no error should be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							{Name: "compile", Variant: model.AllVariants},
						},
					},
					{
						Name:      "testTwo",
						DependsOn: []model.TaskDependency{{Name: model.AllDependencies, Variant: model.AllVariants}},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"}, {Name: "testOne"}, {Name: "testTwo"}},
					},
				},
			}

			So(checkDependencyGraph(project), ShouldResemble, []ValidationError{})
		})

	})
}

func TestVerifyTaskRequirements(t *testing.T) {
	Convey("When validating a project's requirements", t, func() {
		Convey("projects with requirements for non-existing tasks should error", func() {
			p := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "1", Requires: []model.TaskRequirement{{Name: "2"}}},
					{Name: "X"},
				},
				BuildVariants: []model.BuildVariant{
					{Name: "v1", Tasks: []model.BuildVariantTask{
						{Name: "1"},
						{Name: "X", Requires: []model.TaskRequirement{{Name: "2"}}}},
					},
				},
			}
			So(verifyTaskRequirements(p), ShouldNotResemble, []ValidationError{})
			So(len(verifyTaskRequirements(p)), ShouldEqual, 2)
		})
		Convey("projects with requirements for non-existing variants should error", func() {
			p := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "1", Requires: []model.TaskRequirement{{Name: "X", Variant: "$"}}},
					{Name: "X"},
				},
				BuildVariants: []model.BuildVariant{
					{Name: "v1", Tasks: []model.BuildVariantTask{
						{Name: "1"},
						{Name: "X", Requires: []model.TaskRequirement{{Name: "1", Variant: "$"}}}},
					},
				},
			}
			So(verifyTaskRequirements(p), ShouldNotResemble, []ValidationError{})
			So(len(verifyTaskRequirements(p)), ShouldEqual, 2)
		})
		Convey("projects with requirements for a normal project configuration should pass", func() {
			all := []model.BuildVariantTask{{Name: "1"}, {Name: "2"}, {Name: "3"},
				{Name: "before"}, {Name: "after"}}
			beforeDep := []model.TaskDependency{{Name: "before"}}
			p := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "before", Requires: []model.TaskRequirement{{Name: "after"}}},
					{Name: "1", DependsOn: beforeDep},
					{Name: "2", DependsOn: beforeDep},
					{Name: "3", DependsOn: beforeDep},
					{Name: "after", DependsOn: []model.TaskDependency{
						{Name: "before"},
						{Name: "1", PatchOptional: true},
						{Name: "2", PatchOptional: true},
						{Name: "3", PatchOptional: true},
					}},
				},
				BuildVariants: []model.BuildVariant{
					{Name: "v1", Tasks: all},
					{Name: "v2", Tasks: all},
				},
			}
			So(verifyTaskRequirements(p), ShouldResemble, []ValidationError{})
		})
	})
}

func TestValidateBVNames(t *testing.T) {
	Convey("When validating a project's build variants' names", t, func() {
		Convey("if any variant has a duplicate entry, an error should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux"},
					{Name: "linux"},
				},
			}
			validationResults := validateBVNames(project)
			So(validationResults, ShouldNotResemble, []ValidationError{})
			So(len(validationResults), ShouldEqual, 1)
			So(validationResults[0].Level, ShouldEqual, Error)
		})

		Convey("if two variants have the same display name, a warning should be returned, but no errors", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux1", DisplayName: "foo"},
					{Name: "linux", DisplayName: "foo"},
				},
			}

			validationResults := validateBVNames(project)
			numErrors, numWarnings := 0, 0
			for _, val := range validationResults {
				if val.Level == Error {
					numErrors++
				} else if val.Level == Warning {
					numWarnings++
				}
			}

			So(numWarnings, ShouldEqual, 1)
			So(numErrors, ShouldEqual, 0)
			So(len(validationResults), ShouldEqual, 1)
		})

		Convey("if several buildvariants have duplicate entries, all errors "+
			"should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux"},
					{Name: "linux"},
					{Name: "windows"},
					{Name: "windows"},
				},
			}
			So(validateBVNames(project), ShouldNotResemble, []ValidationError{})
			So(len(validateBVNames(project)), ShouldEqual, 2)
		})

		Convey("if no buildvariants have duplicate entries, no error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux"},
					{Name: "windows"},
				},
			}
			So(validateBVNames(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestValidateBVTaskNames(t *testing.T) {
	Convey("When validating a project's build variant's task names", t, func() {
		Convey("if any task has a duplicate entry, an error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"},
							{Name: "compile"},
						},
					},
				},
			}
			So(validateBVTaskNames(project), ShouldNotResemble, []ValidationError{})
			So(len(validateBVTaskNames(project)), ShouldEqual, 1)
		})

		Convey("if several task have duplicate entries, all errors should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"},
							{Name: "compile"},
							{Name: "test"},
							{Name: "test"},
						},
					},
				},
			}
			So(validateBVTaskNames(project), ShouldNotResemble, []ValidationError{})
			So(len(validateBVTaskNames(project)), ShouldEqual, 2)
		})

		Convey("if no tasks have duplicate entries, no error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"},
							{Name: "test"},
						},
					},
				},
			}
			So(validateBVTaskNames(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestCheckAllDependenciesSpec(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("if a task references all dependencies, no other dependency "+
			"should be specified. If one is, an error should be returned",
			func() {
				project := &model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "compile",
							DependsOn: []model.TaskDependency{
								{Name: model.AllDependencies},
								{Name: "testOne"},
							},
						},
					},
				}
				So(checkAllDependenciesSpec(project), ShouldNotResemble,
					[]ValidationError{})
				So(len(checkAllDependenciesSpec(project)), ShouldEqual, 1)
			})
		Convey("if a task references only all dependencies, no error should "+
			"be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						DependsOn: []model.TaskDependency{
							{Name: model.AllDependencies},
						},
					},
				},
			}
			So(checkAllDependenciesSpec(project), ShouldResemble, []ValidationError{})
		})
		Convey("if a task references any other dependencies, no error should "+
			"be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						DependsOn: []model.TaskDependency{
							{Name: "hello"},
						},
					},
				},
			}
			So(checkAllDependenciesSpec(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestValidateProjectTaskNames(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure any duplicate task names throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{Name: "compile"},
				},
			}
			So(validateProjectTaskNames(project), ShouldNotResemble, []ValidationError{})
			So(len(validateProjectTaskNames(project)), ShouldEqual, 1)
		})
		Convey("ensure unique task names do not throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
			}
			So(validateProjectTaskNames(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestValidateProjectTaskIdsAndTags(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure bad task tags throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile", Tags: []string{"a", "!b", "ccc ccc", "d", ".e", "f\tf"}},
				},
			}
			So(validateProjectTaskIdsAndTags(project), ShouldNotResemble, []ValidationError{})
			So(len(validateProjectTaskIdsAndTags(project)), ShouldEqual, 4)
		})
		Convey("ensure bad task names throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{Name: "!compile"},
					{Name: ".compile"},
					{Name: "Fun!"},
				},
			}
			So(validateProjectTaskIdsAndTags(project), ShouldNotResemble, []ValidationError{})
			So(len(validateProjectTaskIdsAndTags(project)), ShouldEqual, 2)
		})
	})
}

func TestCheckTaskCommands(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure tasks that do not have at least one command throw "+
			"an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
			}
			So(checkTaskCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(checkTaskCommands(project)), ShouldEqual, 1)
		})
		Convey("ensure tasks that have at least one command do not throw any errors",
			func() {
				project := &model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "compile",
							Commands: []model.PluginCommandConf{
								{
									Command: "gotest.parse_files",
									Params: map[string]interface{}{
										"files": []interface{}{"test"},
									},
								},
							},
						},
					},
				}
				So(validateProjectTaskNames(project), ShouldResemble, []ValidationError{})
			})
	})
}

func TestEnsureReferentialIntegrity(t *testing.T) {
	Convey("When validating a project", t, func() {
		distroIds := []string{"rhel55"}
		Convey("an error should be thrown if a referenced task for a "+
			"buildvariant does not exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							{Name: "test"},
						},
					},
				},
			}
			So(ensureReferentialIntegrity(project, distroIds), ShouldNotResemble,
				[]ValidationError{})
			So(len(ensureReferentialIntegrity(project, distroIds)), ShouldEqual, 1)
		})

		Convey("no error should be thrown if a referenced task for a "+
			"buildvariant does exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							{Name: "compile"},
						},
					},
				},
			}
			So(ensureReferentialIntegrity(project, distroIds), ShouldResemble,
				[]ValidationError{})
		})

		Convey("an error should be thrown if a referenced distro for a "+
			"buildvariant does not exist", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name:  "enterprise",
						RunOn: []string{"hello"},
					},
				},
			}
			So(ensureReferentialIntegrity(project, distroIds), ShouldNotResemble,
				[]ValidationError{})
			So(len(ensureReferentialIntegrity(project, distroIds)), ShouldEqual, 1)
		})

		Convey("no error should be thrown if a referenced distro for a "+
			"buildvariant does exist", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name:  "enterprise",
						RunOn: []string{"rhel55"},
					},
				},
			}
			So(ensureReferentialIntegrity(project, distroIds), ShouldResemble, []ValidationError{})
		})
	})
}

func TestValidatePluginCommands(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("an error should be thrown if a referenced plugin for a task does not exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "a.b",
								Params:   map[string]interface{}{},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("an error should be thrown if a referenced function command is invalid (invalid params)", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "gotest.parse_files",
							Params: map[string]interface{}{
								"blah": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a function plugin command is valid", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if a function 'a' references "+
			"any another function", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"a": {
						SingleCommand: &model.PluginCommandConf{
							Function: "b",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
					"b": {
						SingleCommand: &model.PluginCommandConf{
							Command: "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("errors should be thrown if a function 'a' references "+
			"another function, 'b', which that does not exist", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"a": {
						SingleCommand: &model.PluginCommandConf{
							Function: "b",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 2)
		})

		Convey("an error should be thrown if a referenced pre plugin command is invalid", func() {
			project := &model.Project{
				Pre: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Command: "gotest.parse_files",
							Params:  map[string]interface{}{},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced pre plugin command is valid", func() {
			project := &model.Project{
				Pre: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if a referenced post plugin command is invalid", func() {
			project := &model.Project{
				Post: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params:   map[string]interface{}{},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced post plugin command is valid", func() {
			project := &model.Project{
				Post: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if a referenced timeout plugin command is invalid", func() {
			project := &model.Project{
				Timeout: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params:   map[string]interface{}{},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced timeout plugin command is valid", func() {
			project := &model.Project{
				Timeout: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}

			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("no error should be thrown if a referenced plugin for a task does exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "archive.targz_pack",
								Params: map[string]interface{}{
									"target":     "tgz",
									"source_dir": "src",
									"include":    []string{":"},
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("no error should be thrown if a referenced plugin that exists contains unneeded parameters", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "archive.targz_pack",
								Params: map[string]interface{}{
									"target":     "tgz",
									"source_dir": "src",
									"include":    []string{":"},
									"extraneous": "G",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if a referenced plugin contains invalid parameters", func() {
			params := map[string]interface{}{
				"aws_key":    "key",
				"aws_secret": "sec",
				"s3_copy_files": []interface{}{
					map[string]interface{}{
						"source": map[string]interface{}{
							"bucket": "long3nough",
							"path":   "fghij",
						},
						"destination": map[string]interface{}{
							"bucket": "..long-but-invalid",
							"path":   "fghij",
						},
					},
				},
			}
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "s3Copy.copy",
								Params:   params,
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced plugin that "+
			"exists contains params that appear invalid but are in expansions",
			func() {
				params := map[string]interface{}{
					"aws_key":    "key",
					"aws_secret": "sec",
					"s3_copy_files": []interface{}{
						map[string]interface{}{
							"source": map[string]interface{}{
								"bucket": "long3nough",
								"path":   "fghij",
							},
							"destination": map[string]interface{}{
								"bucket": "${..longButInvalid}",
								"path":   "fghij",
							},
						},
					},
				}
				project := &model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "compile",
							Commands: []model.PluginCommandConf{
								{
									Function: "",
									Command:  "s3Copy.copy",
									Params:   params,
								},
							},
						},
					},
				}
				So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
			})
		Convey("no error should be thrown if a referenced plugin contains all "+
			"the necessary and valid parameters", func() {
			params := map[string]interface{}{
				"aws_key":    "key",
				"aws_secret": "sec",
				"s3_copy_files": []interface{}{
					map[string]interface{}{
						"source": map[string]interface{}{
							"bucket": "abcde",
							"path":   "fghij",
						},
						"destination": map[string]interface{}{
							"bucket": "abcde",
							"path":   "fghij",
						},
					},
				},
			}
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "s3Copy.copy",
								Params:   params,
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestCheckProjectSyntax(t *testing.T) {
	Convey("When validating a project's syntax", t, func() {
		Convey("if the project passes all of the validation funcs, no errors"+
			" should be returned", func() {
			distros := []distro.Distro{
				{Id: "test-distro-one"},
				{Id: "test-distro-two"},
			}

			err := testutil.CreateTestLocalConfig(projectValidatorConf, "project_test", "")
			So(err, ShouldBeNil)

			projectRef, err := model.FindOneProjectRef("project_test")
			So(err, ShouldBeNil)

			for _, d := range distros {
				So(d.Insert(), ShouldBeNil)
			}

			project, err := model.FindProject("", projectRef)
			So(err, ShouldBeNil)
			verrs, err := CheckProjectSyntax(project)
			So(err, ShouldBeNil)
			So(verrs, ShouldResemble, []ValidationError{})
		})

		Reset(func() {
			db.Clear(distro.Collection)
		})
	})
}

func TestCheckProjectSemantics(t *testing.T) {
	Convey("When validating a project's semantics", t, func() {
		Convey("if the project passes all of the validation funcs, no errors"+
			" should be returned", func() {
			distros := []distro.Distro{
				{Id: "test-distro-one"},
				{Id: "test-distro-two"},
			}

			for _, d := range distros {
				So(d.Insert(), ShouldBeNil)
			}

			projectRef := &model.ProjectRef{
				Identifier:  "project_test",
				LocalConfig: "test: testing",
			}

			project, err := model.FindProject("", projectRef)
			So(err, ShouldBeNil)
			So(CheckProjectSemantics(project), ShouldResemble, []ValidationError{})
		})

		Reset(func() {
			db.Clear(distro.Collection)
		})
	})
}

func TestEnsureHasNecessaryProjectFields(t *testing.T) {
	Convey("When ensuring necessary project fields are set, ensure that", t, func() {
		Convey("projects validate all necessary fields exist", func() {
			Convey("an error should be thrown if the batch_time field is "+
				"set to a negative value", func() {
				project := &model.Project{
					Enabled:     true,
					Identifier:  "identifier",
					Owner:       "owner",
					Repo:        "repo",
					Branch:      "branch",
					DisplayName: "test",
					RepoKind:    "github",
					BatchTime:   -10,
				}
				So(ensureHasNecessaryProjectFields(project),
					ShouldNotResemble, []ValidationError{})
				So(len(ensureHasNecessaryProjectFields(project)),
					ShouldEqual, 1)
			})
			Convey("an error should be thrown if the command type "+
				"field is invalid", func() {
				project := &model.Project{
					BatchTime:   10,
					CommandType: "random",
				}
				So(ensureHasNecessaryProjectFields(project),
					ShouldNotResemble, []ValidationError{})
				So(len(ensureHasNecessaryProjectFields(project)),
					ShouldEqual, 1)
			})
		})
	})
}

func TestEnsureHasNecessaryBVFields(t *testing.T) {
	Convey("When ensuring necessary buildvariant fields are set, ensure that", t, func() {
		Convey("an error is thrown if no build variants exist", func() {
			project := &model.Project{
				Identifier: "test",
			}
			So(ensureHasNecessaryBVFields(project),
				ShouldNotResemble, []ValidationError{})
			So(len(ensureHasNecessaryBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("buildvariants with none of the necessary fields set throw errors", func() {
			project := &model.Project{
				Identifier:    "test",
				BuildVariants: []model.BuildVariant{{}},
			}
			So(ensureHasNecessaryBVFields(project),
				ShouldNotResemble, []ValidationError{})
			So(len(ensureHasNecessaryBVFields(project)),
				ShouldEqual, 2)
		})
		Convey("an error is thrown if the buildvariant does not have a "+
			"name field set", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						RunOn: []string{"mongo"},
						Tasks: []model.BuildVariantTask{{Name: "db"}},
					},
				},
			}
			So(ensureHasNecessaryBVFields(project),
				ShouldNotResemble, []ValidationError{})
			So(len(ensureHasNecessaryBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("an error is thrown if the buildvariant does not have any tasks set", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name:  "postal",
						RunOn: []string{"service"},
					},
				},
			}
			So(ensureHasNecessaryBVFields(project),
				ShouldNotResemble, []ValidationError{})
			So(len(ensureHasNecessaryBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("no error is thrown if the buildvariant has a run_on field set", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name:  "import",
						RunOn: []string{"export"},
						Tasks: []model.BuildVariantTask{{Name: "db"}},
					},
				},
			}
			So(ensureHasNecessaryBVFields(project),
				ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if the buildvariant has no "+
			"run_on field and at least one task has no distro field "+
			"specified", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name:  "import",
						Tasks: []model.BuildVariantTask{{Name: "db"}},
					},
				},
			}
			So(ensureHasNecessaryBVFields(project),
				ShouldNotResemble, []ValidationError{})
			So(len(ensureHasNecessaryBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("no error should be thrown if the buildvariant does not "+
			"have a run_on field specified but all tasks within it have a "+
			"distro field specified", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name: "import",
						Tasks: []model.BuildVariantTask{
							{
								Name: "silhouettes",
								Distros: []string{
									"echoes",
								},
							},
						},
					},
				},
			}
			So(ensureHasNecessaryBVFields(project),
				ShouldResemble, []ValidationError{})
		})
	})
}
