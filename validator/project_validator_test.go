package validator

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	_ "10gen.com/mci/plugin/config"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	projectValidatorConf = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(projectValidatorConf))
}

func TestVerifyTaskDependencies(t *testing.T) {
	Convey("When validating a project's dependencies", t, func() {
		Convey("if any task has a duplicate dependency, an error should be"+
			" returned", func() {

			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					model.ProjectTask{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "compile",
							},
							model.TaskDependency{
								Name: "compile",
							},
						},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldNotResemble, []ValidationError{})
			So(len(verifyTaskDependencies(project)), ShouldEqual, 1)
		})

		Convey("if any dependencies have an invalid name field, an error"+
			" should be returned", func() {

			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					model.ProjectTask{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "bad",
							},
						},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldNotResemble, []ValidationError{})
			So(len(verifyTaskDependencies(project)), ShouldEqual, 1)
		})

		Convey("if the dependencies are well-formed, no error should be"+
			" returned", func() {

			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					model.ProjectTask{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "compile",
							},
						},
					},
					model.ProjectTask{
						Name: "testTwo",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "compile",
							},
						},
					},
				},
			}
			So(verifyTaskDependencies(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestCheckDependencyGraph(t *testing.T) {
	Convey("When checking a project's dependency graph", t, func() {
		Convey("if there is a cycle in the dependency graph, an error should"+
			" be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "testOne",
							},
						},
					},
					model.ProjectTask{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "compile",
							},
						},
					},
					model.ProjectTask{
						Name: "testTwo",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "compile",
							},
						},
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
					model.ProjectTask{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					model.ProjectTask{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "testOne",
							},
						},
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
					model.ProjectTask{
						Name:      "compile",
						DependsOn: []model.TaskDependency{},
					},
					model.ProjectTask{
						Name: "testOne",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "compile",
							},
						},
					},
					model.ProjectTask{
						Name: "testTwo",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "compile",
							},
						},
					},
					model.ProjectTask{
						Name: "push",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "testOne",
							},
							model.TaskDependency{
								Name: "testTwo",
							},
						},
					},
				},
			}
			So(checkDependencyGraph(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestValidateBVNames(t *testing.T) {
	Convey("When validating a project's build variants' names", t, func() {
		Convey("if any buildvariant has a duplicate entry, an error should be "+
			"returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					model.BuildVariant{
						Name: "linux",
					},
					model.BuildVariant{
						Name: "linux",
					},
				},
			}
			So(validateBVNames(project), ShouldNotResemble, []ValidationError{})
			So(len(validateBVNames(project)), ShouldEqual, 1)
		})

		Convey("if several buildvariants have duplicate entries, all errors "+
			"should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					model.BuildVariant{
						Name: "linux",
					},
					model.BuildVariant{
						Name: "linux",
					},
					model.BuildVariant{
						Name: "windows",
					},
					model.BuildVariant{
						Name: "windows",
					},
				},
			}
			So(validateBVNames(project), ShouldNotResemble, []ValidationError{})
			So(len(validateBVNames(project)), ShouldEqual, 2)
		})

		Convey("if no buildvariants have duplicate entries, no error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					model.BuildVariant{
						Name: "linux",
					},
					model.BuildVariant{
						Name: "windows",
					},
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
					model.BuildVariant{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							model.BuildVariantTask{
								Name: "compile",
							},
							model.BuildVariantTask{
								Name: "compile",
							},
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
					model.BuildVariant{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							model.BuildVariantTask{
								Name: "compile",
							},
							model.BuildVariantTask{
								Name: "compile",
							},
							model.BuildVariantTask{
								Name: "test",
							},
							model.BuildVariantTask{
								Name: "test",
							},
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
					model.BuildVariant{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							model.BuildVariantTask{
								Name: "compile",
							},
							model.BuildVariantTask{
								Name: "test",
							},
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
						model.ProjectTask{
							Name: "compile",
							DependsOn: []model.TaskDependency{
								model.TaskDependency{
									Name: "*",
								},
								model.TaskDependency{
									Name: "testOne",
								},
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
					model.ProjectTask{
						Name: "compile",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "*",
							},
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
					model.ProjectTask{
						Name: "compile",
						DependsOn: []model.TaskDependency{
							model.TaskDependency{
								Name: "hello",
							},
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
					model.ProjectTask{
						Name: "compile",
					},
					model.ProjectTask{
						Name: "compile",
					},
				},
			}
			So(validateProjectTaskNames(project), ShouldNotResemble, []ValidationError{})
			So(len(validateProjectTaskNames(project)), ShouldEqual, 1)
		})
		Convey("ensure unique task names do not throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
					},
				},
			}
			So(validateProjectTaskNames(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestCheckTaskCommands(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure tasks that do not have at least one command throw "+
			"an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
					},
				},
			}
			So(checkTaskCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(checkTaskCommands(project)), ShouldEqual, 1)
		})
		Convey("ensure tasks that have at least one command do not throw any errors",
			func() {
				project := &model.Project{
					Tasks: []model.ProjectTask{
						model.ProjectTask{
							Name: "compile",
							Commands: []model.PluginCommandConf{
								model.PluginCommandConf{
									Command: "gotest.run",
									Params: map[string]interface{}{
										"working_dir": "key",
										"tests": []interface{}{
											map[string]interface{}{
												"dir":  "key",
												"args": "sec",
											},
										},
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
		Convey("an error should be thrown if a referenced task for a "+
			"buildvariant does not exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
					},
				},
				BuildVariants: []model.BuildVariant{
					model.BuildVariant{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							model.BuildVariantTask{
								Name: "test",
							},
						},
					},
				},
			}
			So(ensureReferentialIntegrity(project), ShouldNotResemble,
				[]ValidationError{})
			So(len(ensureReferentialIntegrity(project)), ShouldEqual, 1)
		})

		Convey("no error should be thrown if a referenced task for a "+
			"buildvariant does exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
					},
				},
				BuildVariants: []model.BuildVariant{
					model.BuildVariant{
						Name: "linux",
						Tasks: []model.BuildVariantTask{
							model.BuildVariantTask{
								Name: "compile",
							},
						},
					},
				},
			}
			So(ensureReferentialIntegrity(project), ShouldResemble,
				[]ValidationError{})
		})

		Convey("an error should be thrown if a referenced distro for a "+
			"buildvariant does not exist", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					model.BuildVariant{
						Name: "enterprise",
						RunOn: []string{
							"hello",
						},
					},
				},
			}
			So(ensureReferentialIntegrity(project), ShouldNotResemble,
				[]ValidationError{})
			So(len(ensureReferentialIntegrity(project)), ShouldEqual, 1)
		})

		distroNames = []string{"rhel55"}

		Convey("no error should be thrown if a referenced distro for a "+
			"buildvariant does exist", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					model.BuildVariant{
						Name: "enterprise",
						RunOn: []string{
							"rhel55",
						},
					},
				},
			}
			So(ensureReferentialIntegrity(project), ShouldResemble, []ValidationError{})
		})
	})
}

func TestValidatePluginCommands(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("an error should be thrown if a referenced plugin for a "+
			"task does not exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							model.PluginCommandConf{
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
		Convey("an error should be thrown if a referenced function plugin "+
			"command is invalid", func() {
			project := &model.Project{
				Functions: map[string]model.PluginCommandConf{
					"funcOne": model.PluginCommandConf{
						Command: "gotest.run",
						Params:  map[string]interface{}{},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("an error should be thrown if a function plugin command is "+
			"invalid", func() {
			project := &model.Project{
				Functions: map[string]model.PluginCommandConf{
					"funcOne": model.PluginCommandConf{
						Command: "gotest.run",
						Params: map[string]interface{}{
							"working_dir": "key",
							"tests": []interface{}{
								map[string]interface{}{
									"dir":  "key",
									"args": "sec",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("no error should be thrown if a function plugin command "+
			"is valid", func() {
			project := &model.Project{
				Functions: map[string]model.PluginCommandConf{
					"funcOne": model.PluginCommandConf{
						Command: "gotest.run",
						Params: map[string]interface{}{
							"working_dir": "key",
							"tests": []interface{}{
								map[string]interface{}{
									"dir":  "key",
									"args": "sec",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if a function 'a' references "+
			"another function 'b' that does not exists", func() {
			project := &model.Project{
				Functions: map[string]model.PluginCommandConf{
					"funcOne": model.PluginCommandConf{
						Function: "anything",
						Command:  "gotest.run",
						Params: map[string]interface{}{
							"working_dir": "key",
							"tests": []interface{}{
								map[string]interface{}{
									"dir":  "key",
									"args": "sec",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("an error should be thrown if a function 'a' references "+
			"another function 'b' even if 'b' exists", func() {
			project := &model.Project{
				Functions: map[string]model.PluginCommandConf{
					"funcOne": model.PluginCommandConf{
						Function: "funcOne",
						Command:  "gotest.run",
						Params: map[string]interface{}{
							"working_dir": "key",
							"tests": []interface{}{
								map[string]interface{}{
									"dir":  "key",
									"args": "sec",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("an error should be thrown if a referenced pre plugin command "+
			"is invalid", func() {
			project := &model.Project{
				Pre: []model.PluginCommandConf{
					model.PluginCommandConf{
						Command: "gotest.run",
						Params:  map[string]interface{}{},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced pre plugin command "+
			"is valid", func() {
			project := &model.Project{
				Pre: []model.PluginCommandConf{
					model.PluginCommandConf{
						Function: "",
						Command:  "gotest.run",
						Params: map[string]interface{}{
							"working_dir": "key",
							"tests": []interface{}{
								map[string]interface{}{
									"dir":  "key",
									"args": "sec",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if a referenced post plugin command "+
			"is invalid", func() {
			project := &model.Project{
				Post: []model.PluginCommandConf{
					model.PluginCommandConf{
						Function: "",
						Command:  "gotest.run",
						Params:   map[string]interface{}{},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced post plugin command "+
			"is valid", func() {
			project := &model.Project{
				Post: []model.PluginCommandConf{
					model.PluginCommandConf{
						Function: "",
						Command:  "gotest.run",
						Params: map[string]interface{}{
							"working_dir": "key",
							"tests": []interface{}{
								map[string]interface{}{
									"dir":  "key",
									"args": "sec",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("an error should be thrown if a referenced timeout plugin "+
			"command is invalid", func() {
			project := &model.Project{
				Timeout: []model.PluginCommandConf{
					model.PluginCommandConf{
						Function: "",
						Command:  "gotest.run",
						Params:   map[string]interface{}{},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, []ValidationError{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced timeout plugin "+
			"command is valid", func() {
			project := &model.Project{
				Timeout: []model.PluginCommandConf{
					model.PluginCommandConf{
						Function: "",
						Command:  "gotest.run",
						Params: map[string]interface{}{
							"working_dir": "key",
							"tests": []interface{}{
								map[string]interface{}{
									"dir":  "key",
									"args": "sec",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, []ValidationError{})
		})
		Convey("no error should be thrown if a referenced plugin for a "+
			"task does exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							model.PluginCommandConf{
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
		Convey("no error should be thrown if a referenced plugin that "+
			"exists contains some unneeded parameters", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					model.ProjectTask{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							model.PluginCommandConf{
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
		Convey("an error should be thrown if a referenced plugin that "+
			"exists contains necessary but invalid parameters", func() {
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
					model.ProjectTask{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							model.PluginCommandConf{
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
						model.ProjectTask{
							Name: "compile",
							Commands: []model.PluginCommandConf{
								model.PluginCommandConf{
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
					model.ProjectTask{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							model.PluginCommandConf{
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
			project, err := model.FindProject("", "project_test",
				projectValidatorConf.ConfigDir)
			So(err, ShouldBeNil)
			// TODO: fix this after MCI-1926 is completed so we can write a
			// config we want to test against
			So(CheckProjectSyntax(project, mci.TestConfig()),
				ShouldResemble, []ValidationError{})
		})
	})
}

func TestCheckProjectSemantics(t *testing.T) {
	Convey("When validating a project's semantics", t, func() {
		Convey("if the project passes all of the validation funcs, no errors"+
			" should be returned", func() {
			project, err := model.FindProject("", "project_test",
				projectValidatorConf.ConfigDir)
			So(err, ShouldBeNil)
			So(CheckProjectSemantics(project, mci.TestConfig()),
				ShouldResemble, []ValidationError{})
		})
	})
}

func TestEnsureHasNecessaryProjectFields(t *testing.T) {
	Convey("When ensuring necessary project fields are set, ensure that",
		t, func() {
			Convey("projects with none of the necessary fields set should "+
				"throw errors", func() {
				project := &model.Project{
					Enabled: true,
				}
				So(len(EnsureHasNecessaryProjectFields(project)), ShouldEqual,
					5)
			})
			Convey("projects validate all necessary fields exist", func() {
				Convey("an error should be thrown if the identifier field is "+
					"not set", func() {
					project := &model.Project{
						Enabled:     true,
						Owner:       "owner",
						Repo:        "repo",
						Branch:      "branch",
						DisplayName: "test",
						RepoKind:    "github",
					}
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
				Convey("an error should be thrown if the owner field is "+
					"not set", func() {
					project := &model.Project{
						Enabled:     true,
						Identifier:  "identifier",
						Repo:        "repo",
						Branch:      "branch",
						DisplayName: "test",
						RepoKind:    "github",
					}
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
				Convey("an error should be thrown if the repo field is "+
					"not set", func() {
					project := &model.Project{
						Enabled:     true,
						Identifier:  "identifier",
						Owner:       "owner",
						Branch:      "branch",
						DisplayName: "test",
						RepoKind:    "github",
					}
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
				Convey("an error should be thrown if the branch field is "+
					"not set", func() {
					project := &model.Project{
						Enabled:     true,
						Identifier:  "identifier",
						Owner:       "owner",
						Repo:        "repo",
						DisplayName: "test",
						RepoKind:    "github",
					}
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
				Convey("an error should be thrown if the repokind field is "+
					"not set", func() {
					project := &model.Project{
						Enabled:     true,
						Identifier:  "identifier",
						Owner:       "owner",
						Repo:        "repo",
						Branch:      "branch",
						DisplayName: "test",
					}
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
				Convey("an error should be thrown if the repokind field is "+
					"set to an invalid value", func() {
					project := &model.Project{
						Enabled:     true,
						Identifier:  "identifier",
						Owner:       "owner",
						Repo:        "repo",
						Branch:      "branch",
						DisplayName: "test",
						RepoKind:    "superversion",
					}
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
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
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
				Convey("an error should be thrown if the remote_path field is "+
					"not set for a remotely tracked project", func() {
					project := &model.Project{
						Enabled:     true,
						Identifier:  "identifier",
						Owner:       "owner",
						Repo:        "repo",
						Branch:      "branch",
						DisplayName: "test",
						RepoKind:    "github",
						Remote:      true,
						BatchTime:   10,
					}
					So(EnsureHasNecessaryProjectFields(project),
						ShouldNotResemble, []ValidationError{})
					So(len(EnsureHasNecessaryProjectFields(project)),
						ShouldEqual, 1)
				})
			})
		})
}

func TestEnsureHasNecessaryBVFields(t *testing.T) {
	Convey("When ensuring necessary buildvariant fields are set, ensure that",
		t, func() {
			Convey("an error is thrown if no build variants exist", func() {
				project := &model.Project{
					Enabled:    true,
					Identifier: "test",
				}
				So(ensureHasNecessaryBVFields(project),
					ShouldNotResemble, []ValidationError{})
				So(len(ensureHasNecessaryBVFields(project)),
					ShouldEqual, 1)
			})
			Convey("buildvariants with none of the necessary fields set "+
				"throw errors", func() {
				project := &model.Project{
					Enabled:    true,
					Identifier: "test",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{},
					},
				}
				So(ensureHasNecessaryBVFields(project),
					ShouldNotResemble, []ValidationError{})
				So(len(ensureHasNecessaryBVFields(project)),
					ShouldEqual, 2)
			})
			Convey("buildvariants with none of the necessary fields set "+
				"throw errors", func() {
				project := &model.Project{
					Enabled:    true,
					Identifier: "test",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{},
					},
				}
				So(ensureHasNecessaryBVFields(project),
					ShouldNotResemble, []ValidationError{})
				So(len(ensureHasNecessaryBVFields(project)),
					ShouldEqual, 2)
			})
			Convey("an error is thrown if the buildvariant does not have a "+
				"name field set", func() {
				project := &model.Project{
					Enabled:    true,
					Identifier: "projectId",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{
							RunOn: []string{"mongo"},
							Tasks: []model.BuildVariantTask{
								model.BuildVariantTask{
									Name: "db",
								},
							},
						},
					},
				}
				So(ensureHasNecessaryBVFields(project),
					ShouldNotResemble, []ValidationError{})
				So(len(ensureHasNecessaryBVFields(project)),
					ShouldEqual, 1)
			})
			Convey("an error is thrown if the buildvariant does not have any "+
				"tasks set", func() {
				project := &model.Project{
					Enabled:    true,
					Identifier: "projectId",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{
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
			Convey("an error is thrown if the buildvariant does not have any "+
				"tasks set", func() {
				project := &model.Project{
					Enabled:    true,
					Identifier: "projectId",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{
							Name:  "import",
							RunOn: []string{"export"},
						},
					},
				}
				So(ensureHasNecessaryBVFields(project),
					ShouldNotResemble, []ValidationError{})
				So(len(ensureHasNecessaryBVFields(project)),
					ShouldEqual, 1)
			})
			Convey("no error is thrown if the buildvariant has a run_on field "+
				"set", func() {
				project := &model.Project{
					Enabled:    true,
					Identifier: "projectId",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{
							Name:  "import",
							RunOn: []string{"export"},
							Tasks: []model.BuildVariantTask{
								model.BuildVariantTask{
									Name: "db",
								},
							},
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
					Enabled:    true,
					Identifier: "projectId",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{
							Name: "import",
							Tasks: []model.BuildVariantTask{
								model.BuildVariantTask{
									Name: "db",
								},
							},
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
					Enabled:    true,
					Identifier: "projectId",
					BuildVariants: []model.BuildVariant{
						model.BuildVariant{
							Name: "import",
							Tasks: []model.BuildVariantTask{
								model.BuildVariantTask{
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
