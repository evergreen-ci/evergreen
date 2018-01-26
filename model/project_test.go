package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestFindProject(t *testing.T) {

	Convey("When finding a project", t, func() {

		Convey("an error should be thrown if the project ref is nil", func() {
			project, err := FindProject("", nil)
			So(err, ShouldNotBeNil)
			So(project, ShouldBeNil)
		})

		Convey("an error should be thrown if the project ref's identifier is nil", func() {
			projRef := &ProjectRef{
				Identifier: "",
			}
			project, err := FindProject("", projRef)
			So(err, ShouldNotBeNil)
			So(project, ShouldBeNil)
		})

		Convey("if the project file exists and is valid, the project spec within"+
			"should be unmarshalled and returned", func() {
			v := &version.Version{
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
				Identifier: "project_test",
				Requester:  evergreen.RepotrackerVersionRequester,
				Config:     "owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch",
			}
			p := &ProjectRef{
				Identifier: "project_test",
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
			}
			testutil.HandleTestingErr(v.Insert(), t, "failed to insert test version: %v", v)
			_, err := FindProject("", p)
			So(err, ShouldBeNil)

		})

	})

}

func TestGetVariantMappings(t *testing.T) {

	Convey("With a project", t, func() {

		Convey("getting variant mappings should return a map of the build"+
			" variant names to their display names", func() {

			project := &Project{
				BuildVariants: []BuildVariant{
					{
						Name:        "bv1",
						DisplayName: "bv1",
					},
					{
						Name:        "bv2",
						DisplayName: "dsp2",
					},
					{
						Name:        "blecch",
						DisplayName: "blecchdisplay",
					},
				},
			}

			mappings := project.GetVariantMappings()
			So(len(mappings), ShouldEqual, 3)
			So(mappings["bv1"], ShouldEqual, "bv1")
			So(mappings["bv2"], ShouldEqual, "dsp2")
			So(mappings["blecch"], ShouldEqual, "blecchdisplay")

		})

	})

}

func TestGetVariantsWithTask(t *testing.T) {

	Convey("With a project", t, func() {

		project := &Project{
			BuildVariants: []BuildVariant{
				{
					Name:  "bv1",
					Tasks: []BuildVariantTaskUnit{{Name: "suite1"}},
				},
				{
					Name: "bv2",
					Tasks: []BuildVariantTaskUnit{
						{Name: "suite1"},
						{Name: "suite2"},
					},
				},
				{
					Name:  "bv3",
					Tasks: []BuildVariantTaskUnit{{Name: "suite2"}},
				},
			},
		}

		Convey("when getting the build variants where a task applies", func() {

			Convey("it should be run on any build variants where the test is"+
				" specified to run", func() {

				variants := project.GetVariantsWithTask("suite1")
				So(len(variants), ShouldEqual, 2)
				So(util.StringSliceContains(variants, "bv1"), ShouldBeTrue)
				So(util.StringSliceContains(variants, "bv2"), ShouldBeTrue)

				variants = project.GetVariantsWithTask("suite2")
				So(len(variants), ShouldEqual, 2)
				So(util.StringSliceContains(variants, "bv2"), ShouldBeTrue)
				So(util.StringSliceContains(variants, "bv3"), ShouldBeTrue)

			})

		})

	})
}

func TestGetModuleRepoName(t *testing.T) {

	Convey("With a module", t, func() {

		Convey("getting the repo owner and name should return the repo"+
			" field, split at the ':' and removing the .git from"+
			" the end", func() {

			module := &Module{
				Repo: "blecch:owner/repo.git",
			}

			owner, name := module.GetRepoOwnerAndName()
			So(owner, ShouldEqual, "owner")
			So(name, ShouldEqual, "repo")

		})

	})
}

func TestPopulateBVT(t *testing.T) {

	Convey("With a test Project and BuildVariantTaskUnit", t, func() {

		project := &Project{
			Tasks: []ProjectTask{
				{
					Name:            "task1",
					ExecTimeoutSecs: 500,
					Stepback:        boolPtr(false),
					DependsOn:       []TaskUnitDependency{{Name: "other"}},
					Priority:        1000,
					Patchable:       boolPtr(false),
				},
			},
			BuildVariants: []BuildVariant{
				{
					Name:  "test",
					Tasks: []BuildVariantTaskUnit{{Name: "task1", Priority: 5}},
				},
			},
		}

		Convey("updating a BuildVariantTaskUnit with unset fields", func() {
			bvt := project.BuildVariants[0].Tasks[0]
			spec := project.GetSpecForTask("task1")
			So(spec.Name, ShouldEqual, "task1")
			bvt.Populate(spec)

			Convey("should inherit the unset fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.ExecTimeoutSecs, ShouldEqual, 500)
				So(bvt.Stepback, ShouldNotBeNil)
				So(bvt.Patchable, ShouldNotBeNil)
				So(len(bvt.DependsOn), ShouldEqual, 1)

				Convey("but not set fields", func() { So(bvt.Priority, ShouldEqual, 5) })
			})
		})

		Convey("updating a BuildVariantTaskUnit with set fields", func() {
			bvt := BuildVariantTaskUnit{
				Name:            "task1",
				ExecTimeoutSecs: 2,
				Stepback:        boolPtr(true),
				DependsOn:       []TaskUnitDependency{{Name: "task2"}, {Name: "task3"}},
			}
			spec := project.GetSpecForTask("task1")
			So(spec.Name, ShouldEqual, "task1")
			bvt.Populate(spec)

			Convey("should not inherit set fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.ExecTimeoutSecs, ShouldEqual, 2)
				So(bvt.Stepback, ShouldNotBeNil)
				So(*bvt.Stepback, ShouldBeTrue)
				So(len(bvt.DependsOn), ShouldEqual, 2)

				Convey("but unset fields should", func() { So(bvt.Priority, ShouldEqual, 1000) })
			})
		})
	})
}

func TestIgnoresAllFiles(t *testing.T) {
	Convey("With test Project.Ignore setups and a list of.py, .yml, and .md files", t, func() {
		files := []string{
			"src/cool/test.py",
			"etc/other_config.yml",
			"README.md",
		}
		Convey("a project with an empty ignore field should never ignore files", func() {
			p := &Project{Ignore: []string{}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project with a * ignore field should always ignore files", func() {
			p := &Project{Ignore: []string{"*"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores .py files should not ignore all files", func() {
			p := &Project{Ignore: []string{"*.py"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project that ignores .py, .yml, and .md files should ignore all files", func() {
			p := &Project{Ignore: []string{"*.py", "*.yml", "*.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores all files by name should ignore all files", func() {
			p := &Project{Ignore: []string{
				"src/cool/test.py",
				"etc/other_config.yml",
				"README.md",
			}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores all files by dir should ignore all files", func() {
			p := &Project{Ignore: []string{"src/*", "etc/*", "README.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project with negations should not ignore all files", func() {
			p := &Project{Ignore: []string{"*", "!src/cool/*"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project with a negated filetype should not ignore all files", func() {
			p := &Project{Ignore: []string{"src/*", "!*.py", "*yml", "*.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func TestAliasResolution(t *testing.T) {
	assert := assert.New(t) //nolint
	testutil.HandleTestingErr(db.ClearCollections(ProjectVarsCollection), t, "Error clearing collection")
	vars := ProjectVars{
		Id: "project",
		PatchDefinitions: []PatchDefinition{
			{
				Alias:   "all",
				Variant: ".*",
				Task:    ".*",
			},
			{
				Alias:   "bv2",
				Variant: ".*_2",
				Task:    ".*",
			},
			{
				Alias:   "2tasks",
				Variant: ".*",
				Task:    ".*_2",
			},
			{
				Alias:   "aTags",
				Variant: ".*",
				Tags:    []string{"a"},
			},
			{
				Alias:   "aTags_2tasks",
				Variant: ".*",
				Task:    ".*_2",
				Tags:    []string{"a"},
			},
		},
	}
	assert.NoError(vars.Insert())
	p := &Project{
		Identifier: "project",
		BuildVariants: []BuildVariant{
			{
				Name: "bv_1",
				Tasks: []BuildVariantTaskUnit{
					{
						Name: "a_task_1",
					},
					{
						Name: "a_task_2",
					},
					{
						Name: "b_task_1",
					},
					{
						Name: "b_task_2",
					},
				},
			},
			{
				Name: "bv_2",
				Tasks: []BuildVariantTaskUnit{
					{
						Name: "a_task_1",
					},
					{
						Name: "a_task_2",
					},
					{
						Name: "b_task_1",
					},
					{
						Name: "b_task_2",
					},
				},
			},
		},
		Tasks: []ProjectTask{
			{
				Name: "a_task_1",
				Tags: []string{"a", "1"},
			},
			{
				Name: "a_task_2",
				Tags: []string{"a", "2"},
			},
			{
				Name: "b_task_1",
				Tags: []string{"b", "1"},
			},
			{
				Name: "b_task_2",
				Tags: []string{"b", "2"},
			},
		},
	}

	// test that .* on variants and tasks selects everything
	pairs, err := p.BuildProjectTVPairsWithAlias(vars.PatchDefinitions[0].Alias)
	assert.NoError(err)
	assert.Len(pairs, 8)

	// test that the .*_2 regex on variants selects just bv_2
	pairs, err = p.BuildProjectTVPairsWithAlias(vars.PatchDefinitions[1].Alias)
	assert.NoError(err)
	assert.Len(pairs, 4)
	for _, pair := range pairs {
		assert.Equal("bv_2", pair.Variant)
	}

	// test that the .*_2 regex on tasks selects just the _2 tasks
	pairs, err = p.BuildProjectTVPairsWithAlias(vars.PatchDefinitions[2].Alias)
	assert.NoError(err)
	assert.Len(pairs, 4)
	for _, pair := range pairs {
		assert.Contains(pair.TaskName, "task_2")
	}

	// test that the 'a' tag only selects 'a' tasks
	pairs, err = p.BuildProjectTVPairsWithAlias(vars.PatchDefinitions[3].Alias)
	assert.NoError(err)
	assert.Len(pairs, 4)
	for _, pair := range pairs {
		assert.Contains(pair.TaskName, "a_task")
	}

	// test that the 'a' tag and .*_2 regex selects the union of both
	pairs, err = p.BuildProjectTVPairsWithAlias(vars.PatchDefinitions[4].Alias)
	assert.NoError(err)
	assert.Len(pairs, 6)
	for _, pair := range pairs {
		assert.NotEqual("b_task_1", pair.TaskName)
	}
}

func TestGetTaskGroup(t *testing.T) {
	assert := assert.New(t) //nolint
	testutil.HandleTestingErr(db.ClearCollections(version.Collection), t, "failed to clear collections")
	tgName := "example_task_group"
	projYml := `
tasks:
- name: example_task_1
- name: example_task_2
task_groups:
- name: example_task_group
  max_hosts: 2
  setup_group:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_group:
  - command: shell.exec
    params:
      script: "echo teardown_group"
  setup_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  tasks:
  - example_task_1
  - example_task_2
`
	proj, errs := projectFromYAML([]byte(projYml))
	assert.NotNil(proj)
	assert.Empty(errs)
	v := version.Version{
		Id:     "v1",
		Config: projYml,
	}
	assert.NoError(v.Insert())
	t1 := task.Task{
		Id:        "t1",
		TaskGroup: task.FormTaskGroupId(tgName, v.Id),
		Version:   v.Id,
	}

	tg, err := GetTaskGroup(&t1)
	assert.NoError(err)
	assert.Equal(tgName, tg.Name)
	assert.Len(tg.Tasks, 2)
	assert.Equal(2, tg.MaxHosts)
}

func TestPopulateExpansions(t *testing.T) {
	assert := assert.New(t) // nolint

	d := &distro.Distro{
		Id:      "d1",
		WorkDir: "/home/evg",
		Expansions: []distro.Expansion{
			distro.Expansion{
				Key:   "note",
				Value: "huge success",
			},
			distro.Expansion{
				Key:   "cake",
				Value: "truth",
			},
		},
	}
	v := &version.Version{
		Id:                  "v1",
		Branch:              "master",
		Author:              "somebody",
		RevisionOrderNumber: 42,
		//Requester:           evergreen.PatchVersionRequester,
	}
	taskDoc := &task.Task{
		Id:           "t1",
		DisplayName:  "magical task",
		Version:      "v1",
		Execution:    0,
		BuildId:      "b1",
		BuildVariant: "magic",
		Revision:     "0ed7cbd33263043fa95aadb3f6068ef8d076854a",
		Project:      "mci",
	}

	bv := &BuildVariant{
		Expansions: map[string]string{
			"cake":       "lie",
			"github_org": "wut?",
		},
	}

	assert.Panics(func() {
		_ = populateExpansions(nil, v, bv, taskDoc, nil)
	})
	assert.Panics(func() {
		_ = populateExpansions(d, nil, bv, taskDoc, nil)
	})
	assert.Panics(func() {
		_ = populateExpansions(d, v, nil, taskDoc, nil)
	})
	assert.Panics(func() {
		_ = populateExpansions(d, v, bv, nil, nil)
	})

	expansions := populateExpansions(d, v, bv, taskDoc, nil)
	assert.Len(map[string]string(*expansions), 17)
	assert.Equal("0", expansions.Get("execution"))
	assert.Equal("v1", expansions.Get("version_id"))
	assert.Equal("t1", expansions.Get("task_id"))
	assert.Equal("magical task", expansions.Get("task_name"))
	assert.Equal("b1", expansions.Get("build_id"))
	assert.Equal("magic", expansions.Get("build_variant"))
	assert.Equal("/home/evg", expansions.Get("workdir"))
	assert.Equal("0ed7cbd33263043fa95aadb3f6068ef8d076854a", expansions.Get("revision"))
	assert.Equal("mci", expansions.Get("project"))
	assert.Equal("master", expansions.Get("branch_name"))
	assert.Equal("somebody", expansions.Get("author"))
	assert.Equal("d1", expansions.Get("distro_id"))
	assert.True(expansions.Exists("created_at"))
	assert.Equal("42", expansions.Get("revision_order_id"))
	assert.False(expansions.Exists("is_patch"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))
	assert.Equal("lie", expansions.Get("cake"))

	v.Requester = evergreen.PatchVersionRequester
	expansions = populateExpansions(d, v, bv, taskDoc, nil)
	assert.Len(map[string]string(*expansions), 18)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))

	v.Requester = evergreen.GithubPRRequester
	expansions = populateExpansions(d, v, bv, taskDoc, nil)
	assert.Len(map[string]string(*expansions), 18)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))

	patchDoc := &patch.Patch{
		GithubPatchData: patch.GithubPatch{
			PRNumber:  42,
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			Author:    "octocat",
		},
	}

	expansions = populateExpansions(d, v, bv, taskDoc, patchDoc)
	assert.Len(map[string]string(*expansions), 21)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("evergreen", expansions.Get("github_repo"))
	assert.Equal("octocat", expansions.Get("github_author"))
	assert.Equal("42", expansions.Get("github_pr_number"))
	assert.Equal("wut?", expansions.Get("github_org"))
}
